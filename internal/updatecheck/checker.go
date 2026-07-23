package updatecheck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const (
	LatestReleaseEndpoint = "https://api.github.com/repos/kvitrvn/notevault/releases/latest"
	maxResponseBytes      = 1 << 20
)

var strictVersion = regexp.MustCompile(`^v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

type Result struct {
	CurrentVersion  string
	LatestVersion   string
	UpdateAvailable bool
}

type Checker struct {
	client   *http.Client
	endpoint string
}

func New(client *http.Client, endpoint string) Checker {
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	if endpoint == "" {
		endpoint = LatestReleaseEndpoint
	}
	return Checker{client: client, endpoint: endpoint}
}

func (c Checker) Check(ctx context.Context, currentVersion string) (Result, error) {
	result := Result{CurrentVersion: currentVersion}
	if currentVersion == "dev" {
		return result, nil
	}
	current, err := parseVersion(currentVersion)
	if err != nil {
		return result, fmt.Errorf("version courante invalide : %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint, nil)
	if err != nil {
		return result, fmt.Errorf("préparer la vérification : %w", err)
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("User-Agent", "NoteVault")

	response, err := c.client.Do(request)
	if err != nil {
		return result, fmt.Errorf("vérifier la dernière version : %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return result, fmt.Errorf("vérifier la dernière version : statut HTTP %d", response.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, maxResponseBytes))
	if err := decoder.Decode(&release); err != nil {
		return result, fmt.Errorf("décoder la dernière version : %w", err)
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return result, err
	}
	latest, err := parseVersion(release.TagName)
	if err != nil {
		return result, fmt.Errorf("tag de release invalide : %w", err)
	}

	result.LatestVersion = release.TagName
	result.UpdateAvailable = latest.newerThan(current)
	return result, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("décoder la fin de la réponse : %w", err)
	}
	return errors.New("réponse JSON avec contenu supplémentaire")
}

type version [3]string

func parseVersion(value string) (version, error) {
	matches := strictVersion.FindStringSubmatch(value)
	if matches == nil {
		return version{}, fmt.Errorf("%q ne respecte pas vMAJOR.MINOR.PATCH", value)
	}
	return version{matches[1], matches[2], matches[3]}, nil
}

func (v version) newerThan(other version) bool {
	for i := range v {
		if len(v[i]) != len(other[i]) {
			return len(v[i]) > len(other[i])
		}
		if comparison := strings.Compare(v[i], other[i]); comparison != 0 {
			return comparison > 0
		}
	}
	return false
}
