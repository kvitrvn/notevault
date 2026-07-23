package domain

import "time"

type ApplicationMode string

const (
	ApplicationNoVault ApplicationMode = "noVault"
	ApplicationLocked  ApplicationMode = "locked"
	ApplicationReady   ApplicationMode = "ready"
)

type VaultInfo struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Available    bool      `json:"available"`
	Encrypted    bool      `json:"encrypted"`
	Active       bool      `json:"active"`
	LastOpenedAt time.Time `json:"lastOpenedAt"`
}

type ApplicationStatus struct {
	Mode                ApplicationMode `json:"mode"`
	ActiveVault         *VaultInfo      `json:"activeVault,omitempty"`
	RecentVaults        []VaultInfo     `json:"recentVaults"`
	OnboardingDismissed bool            `json:"onboardingDismissed"`
	Version             string          `json:"version"`
}

type UpdateStatus struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	UpdateAvailable bool   `json:"updateAvailable"`
}

type CreateVaultRequest struct {
	Name       string `json:"name"`
	ParentPath string `json:"parentPath"`
	Encrypted  bool   `json:"encrypted"`
	Passphrase string `json:"passphrase,omitempty"`
}
