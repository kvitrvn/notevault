package vault

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/votre-compte/notevault/internal/domain"

	_ "modernc.org/sqlite"
)

const schemaVersion = "1"

type sqliteIndex struct {
	db *sql.DB
}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS notes (
  relative_path TEXT PRIMARY KEY,
  title         TEXT NOT NULL,
  content       TEXT NOT NULL,
  size          INTEGER NOT NULL,
  created_at    INTEGER NOT NULL,
  updated_at    INTEGER NOT NULL
);

CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
  title,
  content,
  tokenize='unicode61'
);

CREATE TABLE IF NOT EXISTS tags (
  relative_path TEXT NOT NULL,
  tag           TEXT NOT NULL,
  PRIMARY KEY (relative_path, tag)
);
CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);

CREATE TABLE IF NOT EXISTS pinned (
  relative_path TEXT PRIMARY KEY,
  pinned_at     INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
`

func newSQLiteIndex(dbPath string) (Index, error) {
	if err := ensureParentDir(dbPath); err != nil {
		return nil, err
	}
	idx, err := openOrRecover(dbPath)
	if err != nil {
		return nil, err
	}
	return idx, nil
}

// openOrRecover tente d'ouvrir l'index ; en cas de schéma incompatible
// ou de base corrompue, il est reconstruit depuis zéro.
func openOrRecover(dbPath string) (Index, error) {
	idx, err := tryOpenIndex(dbPath)
	if err == nil {
		return idx, nil
	}
	// Récupération : on supprime la base et on retente.
	_ = os.Remove(dbPath)
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")
	return tryOpenIndex(dbPath)
}

func tryOpenIndex(dbPath string) (Index, error) {
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("ouvrir l'index SQLite : %w", err)
	}
	// Plusieurs connexions : autorise les lectures concurrentes avec les
	// écritures (watcher, refresh sidebar). Les écritures sont sérialisées
	// en interne par SQLite (WAL + busy_timeout).
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
	// Vérifie d'abord l'intégrité : si la base est corrompue, on
	// demande à l'appelant de la supprimer et de reconstruire.
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check(1)`).Scan(&integrity); err != nil {
		db.Close()
		return nil, fmt.Errorf("contrôle d'intégrité : %w", err)
	}
	if integrity != "ok" {
		db.Close()
		return nil, fmt.Errorf("intégrité invalide : %s", integrity)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialiser le schéma : %w", err)
	}
	idx := &sqliteIndex{db: db}
	if err := idx.ensureSchemaVersion(); err != nil {
		db.Close()
		return nil, err
	}
	if err := idx.validateStructure(); err != nil {
		db.Close()
		return nil, err
	}
	return idx, nil
}

// validateStructure vérifie que les tables FTS5 ont la bonne forme.
// Une base issue d'une version précédente peut contenir une définition
// différente de notes_fts ; on la détecte ici pour forcer une reconstruction.
func (i *sqliteIndex) validateStructure() error {
	row := i.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = ? AND name = ?`, "table", "notes_fts")
	var def string
	if err := row.Scan(&def); err != nil {
		return fmt.Errorf("inspecter notes_fts : %w", err)
	}
	// On refuse toute définition utilisant content= (ancienne version).
	if strings.Contains(def, "content=") {
		return fmt.Errorf("schéma notes_fts obsolète : %s", def)
	}
	// On s'assure qu'un INSERT simple fonctionne.
	tx, err := i.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO notes_fts(rowid, title, content) VALUES(-1, '', '')`); err != nil {
		return fmt.Errorf("test d'écriture FTS : %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE rowid = -1`); err != nil {
		return fmt.Errorf("test de suppression FTS : %w", err)
	}
	return tx.Commit()
}

func ensureParentDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}

func (i *sqliteIndex) ensureSchemaVersion() error {
	var v string
	err := i.db.QueryRow(`SELECT value FROM meta WHERE key = 'schema_version'`).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = i.db.Exec(`INSERT INTO meta(key, value) VALUES('schema_version', ?)`, schemaVersion)
		if err != nil {
			return fmt.Errorf("écrire schema_version : %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("lire schema_version : %w", err)
	}
	if v != schemaVersion {
		return fmt.Errorf("schéma d'index incompatible : %s (attendu %s)", v, schemaVersion)
	}
	return nil
}

func (i *sqliteIndex) Close() error {
	if i.db == nil {
		return nil
	}
	return i.db.Close()
}

func (i *sqliteIndex) Upsert(note domain.Note) error {
	tx, err := i.db.Begin()
	if err != nil {
		return fmt.Errorf("démarrer la transaction : %w", err)
	}
	defer tx.Rollback()

	size := int64(len(note.Content))
	_, err = tx.Exec(`
        INSERT INTO notes (relative_path, title, content, size, created_at, updated_at)
        VALUES (?, ?, ?, ?, ?, ?)
        ON CONFLICT(relative_path) DO UPDATE SET
          title = excluded.title,
          content = excluded.content,
          size = excluded.size,
          created_at = excluded.created_at,
          updated_at = excluded.updated_at
    `,
		note.RelativePath,
		note.Title,
		note.Content,
		size,
		note.CreatedAt.UTC().Unix(),
		note.UpdatedAt.UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("upsert note : %w", err)
	}

	var rowID int64
	if err := tx.QueryRow(`SELECT rowid FROM notes WHERE relative_path = ?`, note.RelativePath).Scan(&rowID); err != nil {
		return fmt.Errorf("récupérer le rowid : %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE rowid = ?`, rowID); err != nil {
		return fmt.Errorf("nettoyer FTS : %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO notes_fts(rowid, title, content) VALUES(?, ?, ?)`, rowID, note.Title, note.Content); err != nil {
		return fmt.Errorf("alimenter FTS : %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM tags WHERE relative_path = ?`, note.RelativePath); err != nil {
		return fmt.Errorf("nettoyer tags : %w", err)
	}
	for _, tag := range uniqueNonEmpty(note.Tags) {
		if _, err := tx.Exec(`INSERT INTO tags(relative_path, tag) VALUES(?, ?)`, note.RelativePath, tag); err != nil {
			return fmt.Errorf("insérer le tag %s : %w", tag, err)
		}
	}

	return tx.Commit()
}

func (i *sqliteIndex) Delete(relativePath string) error {
	tx, err := i.db.Begin()
	if err != nil {
		return fmt.Errorf("démarrer la transaction : %w", err)
	}
	defer tx.Rollback()

	var rowID int64
	err = tx.QueryRow(`SELECT rowid FROM notes WHERE relative_path = ?`, relativePath).Scan(&rowID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("chercher la note : %w", err)
	}

	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE rowid = ?`, rowID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tags WHERE relative_path = ?`, relativePath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM pinned WHERE relative_path = ?`, relativePath); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes WHERE rowid = ?`, rowID); err != nil {
		return fmt.Errorf("supprimer la note : %w", err)
	}
	return tx.Commit()
}

func (i *sqliteIndex) ListPaths() ([]string, error) {
	rows, err := i.db.Query(`SELECT relative_path FROM notes ORDER BY relative_path`)
	if err != nil {
		return nil, fmt.Errorf("lister les chemins indexés : %w", err)
	}
	defer rows.Close()

	paths := make([]string, 0)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, rows.Err()
}

func (i *sqliteIndex) SetMeta(key, value string) error {
	_, err := i.db.Exec(`
        INSERT INTO meta(key, value) VALUES(?, ?)
        ON CONFLICT(key) DO UPDATE SET value = excluded.value
    `, key, value)
	if err != nil {
		return fmt.Errorf("écrire meta %s : %w", key, err)
	}
	return nil
}

func (i *sqliteIndex) Get(relativePath string) (domain.Note, error) {
	row := i.db.QueryRow(`SELECT title, content, created_at, updated_at FROM notes WHERE relative_path = ?`, relativePath)
	note := domain.Note{RelativePath: relativePath, Tags: []string{}}
	var created, updated int64
	if err := row.Scan(&note.Title, &note.Content, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.Note{}, ErrNotFound
		}
		return domain.Note{}, err
	}
	note.CreatedAt = unixToTime(created)
	note.UpdatedAt = unixToTime(updated)
	tagRows, err := i.db.Query(`SELECT tag FROM tags WHERE relative_path = ? ORDER BY tag`, relativePath)
	if err != nil {
		return domain.Note{}, err
	}
	defer tagRows.Close()
	for tagRows.Next() {
		var t string
		if err := tagRows.Scan(&t); err != nil {
			return domain.Note{}, err
		}
		note.Tags = append(note.Tags, t)
	}
	return note, nil
}

func (i *sqliteIndex) List(filter ListFilter) ([]domain.NoteSummary, error) {
	rows, err := i.queryList(filter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]domain.NoteSummary, 0)
	for rows.Next() {
		var s domain.NoteSummary
		var updated int64
		if err := rows.Scan(&s.RelativePath, &s.Title, &updated); err != nil {
			return nil, err
		}
		s.UpdatedAt = unixToTime(updated)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (i *sqliteIndex) queryList(filter ListFilter) (*sql.Rows, error) {
	limit := clampLimit(filter.Limit)
	conds := make([]string, 0, 6)
	args := make([]any, 0, 8)

	// Préfixe de dossier (notes/projects/...).
	if filter.Folder != "" {
		prefix := strings.TrimSuffix(filter.Folder, "/") + "/"
		conds = append(conds, "(n.relative_path LIKE ? OR n.relative_path = ?)")
		args = append(args, prefix+"%", strings.TrimSuffix(filter.Folder, "/"))
	}

	// Tag unique (legacy).
	if filter.Tag != "" {
		conds = append(conds, "EXISTS (SELECT 1 FROM tags t WHERE t.relative_path = n.relative_path AND t.tag = ?)")
		args = append(args, filter.Tag)
	}

	// Plusieurs tags requis (AND).
	for _, tag := range filter.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		conds = append(conds, "EXISTS (SELECT 1 FROM tags t WHERE t.relative_path = n.relative_path AND t.tag = ?)")
		args = append(args, tag)
	}

	// Tags exclus (NOT EXISTS pour chaque).
	for _, tag := range filter.ExcludeTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		conds = append(conds, "NOT EXISTS (SELECT 1 FROM tags t WHERE t.relative_path = n.relative_path AND t.tag = ?)")
		args = append(args, tag)
	}

	// Filtre temporel updated_at.
	if !filter.UpdatedFrom.IsZero() {
		conds = append(conds, "n.updated_at >= ?")
		args = append(args, filter.UpdatedFrom.UTC().Unix())
	}
	if !filter.UpdatedTo.IsZero() {
		conds = append(conds, "n.updated_at < ?")
		args = append(args, filter.UpdatedTo.UTC().Unix())
	}

	// Recherche full-text (jointure FTS5 si Query).
	joinFTS := ""
	if q := sanitizeFTSQuery(filter.Query); q != "" {
		joinFTS = "JOIN notes_fts f ON f.rowid = n.rowid"
		conds = append(conds, "notes_fts MATCH ?")
		args = append(args, q)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)

	q := `SELECT n.relative_path, n.title, n.updated_at
        FROM notes n
        ` + joinFTS + `
        ` + where + `
        ORDER BY n.updated_at DESC
        LIMIT ?`
	return i.db.Query(q, args...)
}

func (i *sqliteIndex) Search(query string, opts SearchOpts) ([]domain.NoteSummary, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return i.List(ListFilter{Limit: clampLimit(opts.Limit)})
	}
	ftsQuery := sanitizeFTSQuery(query)
	if ftsQuery == "" {
		return i.List(ListFilter{Limit: clampLimit(opts.Limit)})
	}
	limit := clampLimit(opts.Limit)
	rows, err := i.db.Query(`
        SELECT n.relative_path, n.title, n.updated_at
        FROM notes n
        JOIN notes_fts f ON f.rowid = n.rowid
        WHERE notes_fts MATCH ?
        ORDER BY rank
        LIMIT ?
    `, ftsQuery, limit)
	if err != nil {
		return nil, fmt.Errorf("recherche FTS : %w", err)
	}
	defer rows.Close()
	out := make([]domain.NoteSummary, 0)
	for rows.Next() {
		var s domain.NoteSummary
		var updated int64
		if err := rows.Scan(&s.RelativePath, &s.Title, &updated); err != nil {
			return nil, err
		}
		s.UpdatedAt = unixToTime(updated)
		out = append(out, s)
	}
	return out, rows.Err()
}

func (i *sqliteIndex) ListTags() ([]TagCount, error) {
	rows, err := i.db.Query(`SELECT tag, COUNT(*) AS n FROM tags GROUP BY tag ORDER BY n DESC, tag ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TagCount, 0)
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (i *sqliteIndex) ListFolders() ([]FolderInfo, error) {
	rows, err := i.db.Query(`
        SELECT relative_path FROM notes ORDER BY relative_path
    `)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	counts := make(map[string]int)
	parents := make(map[string]struct{})
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		// p = "notes/projects/web/index.md" → dossier = "projects/web"
		parts := strings.SplitN(p, "/", 2)
		if len(parts) < 2 {
			continue
		}
		dir := parts[1]
		// Retire le nom de fichier final.
		if idx := strings.LastIndex(dir, "/"); idx >= 0 {
			dir = dir[:idx]
		} else {
			dir = ""
		}
		if dir == "" {
			continue
		}
		counts[dir]++
		// Conserve les dossiers parents aussi.
		parent := dir
		for {
			if idx := strings.LastIndex(parent, "/"); idx >= 0 {
				parent = parent[:idx]
			} else {
				break
			}
			if parent == "" {
				break
			}
			parents[parent] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Tous les dossiers connus : ceux qui contiennent des notes + leurs parents.
	all := make(map[string]struct{}, len(counts)+len(parents))
	for k := range counts {
		all[k] = struct{}{}
	}
	for k := range parents {
		all[k] = struct{}{}
	}
	out := make([]FolderInfo, 0, len(all))
	for p := range all {
		name := p
		if idx := strings.LastIndex(p, "/"); idx >= 0 {
			name = p[idx+1:]
		}
		out = append(out, FolderInfo{Path: p, Name: name, Count: counts[p]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (i *sqliteIndex) Pin(relativePath string, pinned bool) error {
	if pinned {
		_, err := i.db.Exec(`
            INSERT INTO pinned(relative_path, pinned_at) VALUES(?, ?)
            ON CONFLICT(relative_path) DO UPDATE SET pinned_at = excluded.pinned_at
        `, relativePath, nowUTC().Unix())
		if err != nil {
			return fmt.Errorf("épingler : %w", err)
		}
		return nil
	}
	if _, err := i.db.Exec(`DELETE FROM pinned WHERE relative_path = ?`, relativePath); err != nil {
		return fmt.Errorf("désépingler : %w", err)
	}
	return nil
}

func (i *sqliteIndex) IsPinned(relativePath string) (bool, error) {
	var n int
	err := i.db.QueryRow(`SELECT COUNT(*) FROM pinned WHERE relative_path = ?`, relativePath).Scan(&n)
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (i *sqliteIndex) ListPinned() ([]domain.NoteSummary, error) {
	rows, err := i.db.Query(`
        SELECT n.relative_path, n.title, n.updated_at
        FROM notes n
        INNER JOIN pinned p ON p.relative_path = n.relative_path
        ORDER BY p.pinned_at DESC
    `)
	if err != nil {
		return nil, fmt.Errorf("lister les épinglées : %w", err)
	}
	defer rows.Close()
	out := make([]domain.NoteSummary, 0)
	for rows.Next() {
		var s domain.NoteSummary
		var updated int64
		if err := rows.Scan(&s.RelativePath, &s.Title, &updated); err != nil {
			return nil, err
		}
		s.UpdatedAt = unixToTime(updated)
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetBacklinks retourne les notes dont le contenu (ou titre) contient la
// phrase exacte donnée. Utilisé pour la recherche de backlinks à partir
// d'un titre de note.
func (i *sqliteIndex) GetBacklinks(title string, opts SearchOpts) ([]domain.NoteSummary, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, nil
	}
	// FTS5 phrase query : on entoure de guillemets doubles, on échappe
	// les guillemets internes. Si le titre est vide après nettoyage,
	// on retourne rien.
	escaped := strings.ReplaceAll(title, `"`, `""`)
	ftsQuery := `"` + escaped + `"`
	limit := clampLimit(opts.Limit)
	var rows *sql.Rows
	var err error
	if opts.ExcludePath != "" {
		rows, err = i.db.Query(`
            SELECT n.relative_path, n.title, n.updated_at
            FROM notes n
            JOIN notes_fts f ON f.rowid = n.rowid
            WHERE notes_fts MATCH ? AND n.relative_path <> ?
            ORDER BY rank
            LIMIT ?
        `, ftsQuery, opts.ExcludePath, limit)
	} else {
		rows, err = i.db.Query(`
            SELECT n.relative_path, n.title, n.updated_at
            FROM notes n
            JOIN notes_fts f ON f.rowid = n.rowid
            WHERE notes_fts MATCH ?
            ORDER BY rank
            LIMIT ?
        `, ftsQuery, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("recherche backlinks : %w", err)
	}
	defer rows.Close()
	out := make([]domain.NoteSummary, 0)
	for rows.Next() {
		var s domain.NoteSummary
		var updated int64
		if err := rows.Scan(&s.RelativePath, &s.Title, &updated); err != nil {
			return nil, err
		}
		s.UpdatedAt = unixToTime(updated)
		out = append(out, s)
	}
	return out, rows.Err()
}

func clampLimit(n int) int {
	if n <= 0 {
		return 1000
	}
	if n > 5000 {
		return 5000
	}
	return n
}

// sanitizeFTSQuery neutralise les caractères spéciaux FTS5 et échappe
// les termes pour permettre une recherche "souple" multi-mots.
// Les caractères réservés (":^"*()") sont remplacés par des espaces.
func sanitizeFTSQuery(s string) string {
	const reserved = ":^\"*()"
	var b strings.Builder
	prevSpace := true
	for _, r := range s {
		if strings.ContainsRune(reserved, r) || r == '\n' || r == '\t' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return strings.TrimSpace(b.String())
}

func uniqueNonEmpty(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func unixToTime(sec int64) time.Time {
	return time.Unix(sec, 0).UTC()
}

// StatsBuckets calcule les agrégats d'activité : compteurs par jour
// (création, modification) sur la fenêtre demandée, totaux (notes, mots,
// top tags). Les jours sont indexés du plus ancien au plus récent.
//
// created_at/updated_at sont stockés en secondes UNIX (UTC). On calcule
// le début de la fenêtre en secondes UNIX minuit UTC et on aligne les
// jours via la fonction date() de SQLite.
func (i *sqliteIndex) StatsBuckets(windowDays int) (StatsBucketsResult, error) {
	if windowDays <= 0 {
		windowDays = 30
	}
	now := nowUTC()
	// Aligne sur minuit UTC du jour courant.
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	start := today.AddDate(0, 0, -windowDays+1)
	cutoff := start.Unix()

	created, err := i.dayBuckets("created_at", cutoff)
	if err != nil {
		return StatsBucketsResult{}, err
	}
	modified, err := i.dayBuckets("updated_at", cutoff)
	if err != nil {
		return StatsBucketsResult{}, err
	}

	var totalNotes int
	if err := i.db.QueryRow(`SELECT COUNT(*) FROM notes`).Scan(&totalNotes); err != nil {
		return StatsBucketsResult{}, fmt.Errorf("compter les notes : %w", err)
	}

	words, err := i.totalWordCount()
	if err != nil {
		return StatsBucketsResult{}, err
	}

	tags, err := i.topTags(10)
	if err != nil {
		return StatsBucketsResult{}, err
	}

	return StatsBucketsResult{
		Created:  created,
		Modified: modified,
		Notes:    totalNotes,
		Words:    words,
		TopTags:  tags,
	}, nil
}

// dayBuckets retourne les compteurs par jour pour une colonne temporelle
// donnée. Les jours sans activité sont omis : le rendu côté frontend
// comble les trous avec des 0.
func (i *sqliteIndex) dayBuckets(column string, cutoff int64) ([]DayCount, error) {
	rows, err := i.db.Query(fmt.Sprintf(`
        SELECT date(%s, 'unixepoch') AS day, COUNT(*) AS n
        FROM notes
        WHERE %s >= ?
        GROUP BY day
        ORDER BY day
    `, column, column), cutoff)
	if err != nil {
		return nil, fmt.Errorf("compteurs par jour : %w", err)
	}
	defer rows.Close()
	out := make([]DayCount, 0, 32)
	for rows.Next() {
		var d DayCount
		if err := rows.Scan(&d.Day, &d.Count); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// totalWordCount additionne les compteurs de mots sur toutes les notes.
// On itère en Go (le SQL n'a pas de tokenizer Markdown natif), mais le
// coût reste linéaire en nombre de notes et borné à < 100 ms même pour
// 10 000 notes.
func (i *sqliteIndex) totalWordCount() (int, error) {
	rows, err := i.db.Query(`SELECT content FROM notes`)
	if err != nil {
		return 0, fmt.Errorf("lire les contenus : %w", err)
	}
	defer rows.Close()
	var total int
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return 0, err
		}
		total += countWords(c)
	}
	return total, rows.Err()
}

func (i *sqliteIndex) topTags(limit int) ([]TagCount, error) {
	rows, err := i.db.Query(`
        SELECT tag, COUNT(*) AS n
        FROM tags
        GROUP BY tag
        ORDER BY n DESC, tag ASC
        LIMIT ?
    `, limit)
	if err != nil {
		return nil, fmt.Errorf("top tags : %w", err)
	}
	defer rows.Close()
	out := make([]TagCount, 0, limit)
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Count); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
