package vault

import (
	"fmt"
	"strings"
)

// unifiedDiff produit un diff unifié simple (sans dépendance externe).
// Assez bon pour un affichage en lecture ; pas adapté à des patches.
func unifiedDiff(aLabel, a, bLabel, b string) string {
	aLines := strings.Split(a, "\n")
	bLines := strings.Split(b, "\n")
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n", aLabel)
	fmt.Fprintf(&sb, "+++ %s\n", bLabel)
	// LCS pour calculer les opérations de diff.
	lcs := lcsTable(aLines, bLines)
	var ops []diffOp
	for i, j := len(aLines), len(bLines); i > 0 || j > 0; {
		switch {
		case i > 0 && j > 0 && aLines[i-1] == bLines[j-1]:
			ops = append(ops, diffOp{kind: ' ', line: aLines[i-1]})
			i--
			j--
		case i > 0 && (j == 0 || lcs[i-1][j] >= lcs[i][j-1]):
			ops = append(ops, diffOp{kind: '-', line: aLines[i-1]})
			i--
		default:
			ops = append(ops, diffOp{kind: '+', line: bLines[j-1]})
			j--
		}
	}
	// Inverse (le balayage ci-dessus produit l'ordre inverse).
	for left, right := 0, len(ops)-1; left < right; left, right = left+1, right-1 {
		ops[left], ops[right] = ops[right], ops[left]
	}
	// Regroupe en hunks de 3 lignes de contexte.
	return renderHunks(ops)
}

type diffOp struct {
	kind byte // ' ' | '+' | '-'
	line string
}

func lcsTable(a, b []string) [][]int {
	la, lb := len(a), len(b)
	tab := make([][]int, la+1)
	for i := range tab {
		tab[i] = make([]int, lb+1)
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] {
				tab[i][j] = tab[i-1][j-1] + 1
			} else if tab[i-1][j] >= tab[i][j-1] {
				tab[i][j] = tab[i-1][j]
			} else {
				tab[i][j] = tab[i][j-1]
			}
		}
	}
	return tab
}

func renderHunks(ops []diffOp) string {
	const ctx = 3
	if len(ops) == 0 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' && i+1 >= len(ops) {
			break
		}
		start := i
		for i < len(ops) && (ops[i].kind != ' ' || (i-start) < ctx) {
			i++
		}
		end := i
		// Étend vers l'arrière pour le contexte.
		ctxStart := start
		for k := 0; k < ctx && ctxStart > 0; k++ {
			if ops[ctxStart-1].kind == ' ' {
				ctxStart--
			} else {
				break
			}
		}
		ctxEnd := end
		for k := 0; k < ctx && ctxEnd < len(ops); k++ {
			if ops[ctxEnd].kind == ' ' {
				ctxEnd++
			} else {
				break
			}
		}
		hunk := ops[ctxStart:ctxEnd]
		if anyChange(hunk) {
			fmt.Fprintf(&sb, "@@ -%d,%d +%d,%d @@\n",
				lineIndex(ops, ctxStart, '-'),
				hunkCount(ops, ctxStart, ctxEnd, '-'),
				lineIndex(ops, ctxStart, '+'),
				hunkCount(ops, ctxStart, ctxEnd, '+'),
			)
			for _, op := range hunk {
				sb.WriteByte(op.kind)
				sb.WriteString(op.line)
				sb.WriteByte('\n')
			}
		}
		i = end
	}
	if sb.Len() == 0 {
		return "(aucune différence)"
	}
	return sb.String()
}

func anyChange(ops []diffOp) bool {
	for _, op := range ops {
		if op.kind != ' ' {
			return true
		}
	}
	return false
}

func hunkCount(ops []diffOp, start, end int, kind byte) int {
	n := 0
	for i := start; i < end; i++ {
		if ops[i].kind == kind || ops[i].kind == ' ' {
			n++
		}
	}
	return n
}

func lineIndex(ops []diffOp, idx int, kind byte) int {
	// Compte les lignes "source" jusqu'à idx (incluse si elle est '-' ou ' ').
	// Pour le '+', compte les lignes destination.
	n := 1
	if kind == '-' {
		for i := 0; i < idx; i++ {
			if ops[i].kind == '-' || ops[i].kind == ' ' {
				n++
			}
		}
	} else {
		for i := 0; i < idx; i++ {
			if ops[i].kind == '+' || ops[i].kind == ' ' {
				n++
			}
		}
	}
	return n
}
