package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kvitrvn/notevault/internal/appconfig"
	"github.com/kvitrvn/notevault/internal/chat"
	"github.com/kvitrvn/notevault/internal/domain"
	"github.com/kvitrvn/notevault/internal/vault"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
)

type fakeAppSecretStore struct {
	keys           map[chat.Provider]string
	availableErr   error
	availableCalls int
	getCalls       int
}

func (f *fakeAppSecretStore) Available() error {
	f.availableCalls++
	return f.availableErr
}

func (f *fakeAppSecretStore) Get(provider chat.Provider) (string, error) {
	f.getCalls++
	if f.availableErr != nil {
		return "", f.availableErr
	}
	key := f.keys[provider]
	if key == "" {
		return "", chat.ErrAPIKeyNotFound
	}
	return key, nil
}

func (f *fakeAppSecretStore) Set(provider chat.Provider, apiKey string) error {
	if f.availableErr != nil {
		return f.availableErr
	}
	if f.keys == nil {
		f.keys = make(map[chat.Provider]string)
	}
	f.keys[provider] = apiKey
	return nil
}

func (f *fakeAppSecretStore) Delete(provider chat.Provider) error {
	if f.availableErr != nil {
		return f.availableErr
	}
	delete(f.keys, provider)
	return nil
}

func TestNewAppDoesNotAccessKeyringAtStartup(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	secrets := &fakeAppSecretStore{keys: make(map[chat.Provider]string)}
	app, err := newApp(appOptions{
		configPath: filepath.Join(root, "config", "app.json"),
		legacyPath: filepath.Join(root, "legacy"),
		now:        time.Now,
		secrets:    secrets,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	if secrets.availableCalls != 0 || secrets.getCalls != 0 {
		t.Fatalf("accès au démarrage: available=%d get=%d", secrets.availableCalls, secrets.getCalls)
	}
}

func setupAppForTest(t *testing.T) *App {
	t.Helper()
	service, err := vault.New(vault.Options{Root: t.TempDir()})
	if err != nil {
		t.Fatalf("vault.New: %v", err)
	}
	srv := vault.NewAssetServer(service.Root())
	if _, err := srv.Start(); err != nil {
		t.Fatalf("asset server start: %v", err)
	}
	t.Cleanup(func() { _ = srv.Stop() })
	store := appconfig.NewStore(filepath.Join(t.TempDir(), "app.json"))
	app := &App{
		session: &vaultSession{service: service, assetSrv: srv, assetPort: 43125},
		config:  appconfig.Default(),
		store:   store,
		now:     time.Now,
		secrets: &fakeAppSecretStore{keys: make(map[chat.Provider]string)},
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	return app
}

func TestAppAssetURL(t *testing.T) {
	app := setupAppForTest(t)
	path := filepath.Join(app.session.service.Root(), "assets", "mes photos", "image.png")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("image"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := app.AssetURL("assets/mes photos/image.png")
	if err != nil {
		t.Fatal(err)
	}
	want := "http://127.0.0.1:43125/files/assets/mes%20photos/image.png?t=" + app.session.assetSrv.Token()
	if got != want {
		t.Fatalf("AssetURL() = %q, want %q", got, want)
	}
}

func TestAppAssetMethodsRejectTraversal(t *testing.T) {
	app := setupAppForTest(t)
	outside := filepath.Join(filepath.Dir(app.session.service.Root()), "outside.png")
	if err := os.WriteFile(outside, []byte("private"), 0o644); err != nil {
		t.Fatal(err)
	}

	for name, call := range map[string]func() error{
		"OpenAsset": func() error {
			_, err := app.OpenAsset("assets/../../outside.png")
			return err
		},
		"AssetURL": func() error {
			_, err := app.AssetURL("assets/../../outside.png")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil || !strings.Contains(err.Error(), "asset") {
				t.Fatalf("call returned %v, want asset validation error", err)
			}
		})
	}
}

func TestNewAppFirstLaunchDoesNotCreateLegacyVault(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "NoteVault")
	app, err := newApp(appOptions{
		configPath: filepath.Join(root, "config", "app.json"),
		legacyPath: legacy,
		now:        time.Now,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	if _, err := os.Stat(legacy); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("le coffre historique a été créé: %v", err)
	}
	status, err := app.ApplicationStatus()
	if err != nil || status.Mode != domain.ApplicationNoVault {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if _, err := app.ListNotes(); !errors.Is(err, ErrNoVaultOpen) {
		t.Fatalf("ListNotes error = %v", err)
	}
}

func TestNewAppMigratesUsedLegacyVaultAndIgnoresEmptyTree(t *testing.T) {
	for _, tc := range []struct {
		name string
		used bool
		mode domain.ApplicationMode
	}{
		{name: "empty", mode: domain.ApplicationNoVault},
		{name: "used", used: true, mode: domain.ApplicationReady},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			legacy := filepath.Join(root, "NoteVault")
			service, err := vault.New(vault.Options{Root: legacy})
			if err != nil {
				t.Fatal(err)
			}
			if tc.used {
				if _, err := service.CreateNote("Importée", ""); err != nil {
					t.Fatal(err)
				}
			}
			_ = service.Close()
			app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: legacy, now: time.Now})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { app.Shutdown(t.Context()) })
			status, _ := app.ApplicationStatus()
			if status.Mode != tc.mode {
				t.Fatalf("mode = %s, want %s", status.Mode, tc.mode)
			}
		})
	}
}

func TestNewAppMigratesOnboardingDismissal(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "NoteVault")
	if err := os.MkdirAll(filepath.Join(legacy, ".notevault"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacy, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, ".notevault", "state.json"), []byte(`{"onboardingCompleted":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: legacy, now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	status, _ := app.ApplicationStatus()
	if !status.OnboardingDismissed {
		t.Fatal("préférence d’onboarding non migrée")
	}
}

func TestAppCreateOpenForgetAndPreserveSessionOnFailure(t *testing.T) {
	root := t.TempDir()
	app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: filepath.Join(root, "legacy"), now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })

	if err := app.CreateVault(domain.CreateVaultRequest{Name: "Clair", ParentPath: root}); err != nil {
		t.Fatal(err)
	}
	firstPath, err := app.VaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := app.OpenVault(filepath.Join(root, "invalide")); err == nil {
		t.Fatal("ouverture invalide acceptée")
	}
	stillActive, _ := app.VaultPath()
	if stillActive != firstPath {
		t.Fatalf("coffre actif changé après échec: %q", stillActive)
	}
	if err := app.ForgetRecentVault(firstPath); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("ForgetRecentVault a touché les fichiers: %v", err)
	}
	status, _ := app.ApplicationStatus()
	if status.ActiveVault == nil || status.ActiveVault.Path != firstPath || len(status.RecentVaults) != 0 {
		t.Fatalf("status après oubli = %+v", status)
	}
}

func TestAppCreateEncryptedVaultOpensLocked(t *testing.T) {
	root := t.TempDir()
	app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: filepath.Join(root, "legacy"), now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	if err := app.CreateVault(domain.CreateVaultRequest{
		Name: "Secret", ParentPath: root, Encrypted: true, Passphrase: "phrase secrète robuste",
	}); err != nil {
		t.Fatal(err)
	}
	status, _ := app.ApplicationStatus()
	if status.Mode != domain.ApplicationLocked || status.ActiveVault == nil || !status.ActiveVault.Encrypted {
		t.Fatalf("status = %+v", status)
	}
}

func TestAppSwitchStopsPreviousAssetServer(t *testing.T) {
	root := t.TempDir()
	app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: filepath.Join(root, "legacy"), now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	if err := app.CreateVault(domain.CreateVaultRequest{Name: "Premier", ParentPath: root}); err != nil {
		t.Fatal(err)
	}
	old := app.session
	if old.assetSrv.Port() == 0 {
		t.Fatal("serveur initial non démarré")
	}
	if err := app.CreateVault(domain.CreateVaultRequest{Name: "Second", ParentPath: root}); err != nil {
		t.Fatal(err)
	}
	if old.assetSrv.Port() != 0 {
		t.Fatal("ancien serveur d’assets encore actif")
	}
}

func TestAppCreateVaultCleansTemporaryDirectoryAfterError(t *testing.T) {
	root := t.TempDir()
	app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: filepath.Join(root, "legacy"), now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	err = app.CreateVault(domain.CreateVaultRequest{Name: "Secret", ParentPath: root, Encrypted: true, Passphrase: "court"})
	if err == nil {
		t.Fatal("phrase secrète invalide acceptée")
	}
	matches, globErr := filepath.Glob(filepath.Join(root, ".notevault-create-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("dossiers temporaires résiduels: %v", matches)
	}
}

func TestAppKeepsCurrentVaultWhenGlobalConfigSaveFails(t *testing.T) {
	root := t.TempDir()
	app, err := newApp(appOptions{configPath: filepath.Join(root, "cfg", "app.json"), legacyPath: filepath.Join(root, "legacy"), now: time.Now})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { app.Shutdown(t.Context()) })
	if err := app.CreateVault(domain.CreateVaultRequest{Name: "Stable", ParentPath: root}); err != nil {
		t.Fatal(err)
	}
	stable, _ := app.VaultPath()
	blocker := filepath.Join(root, "not-a-directory")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	app.store = appconfig.NewStore(filepath.Join(blocker, "app.json"))
	if err := app.CreateVault(domain.CreateVaultRequest{Name: "NonActive", ParentPath: root}); err == nil {
		t.Fatal("échec d’écriture de configuration non propagé")
	}
	active, _ := app.VaultPath()
	if active != stable {
		t.Fatalf("coffre actif = %q, want %q", active, stable)
	}
}

func TestAppReturnsStableSwitchingError(t *testing.T) {
	app := setupAppForTest(t)
	app.switching.Store(true)
	defer app.switching.Store(false)
	if _, err := app.ListNotes(); !errors.Is(err, ErrVaultSwitching) {
		t.Fatalf("error = %v, want ErrVaultSwitching", err)
	}
}

func TestAppChatRefusesEncryptedVaultWithoutCreatingDerivedState(t *testing.T) {
	app := setupAppForTest(t)
	if err := app.session.service.EnableEncryption("phrase secrète robuste"); err != nil {
		t.Fatalf("EnableEncryption: %v", err)
	}

	_, err := app.PrepareChat(chat.PrepareRequest{
		NotePaths: []string{"notes/inconnue.md"},
		Question:  "Que contient la note ?",
		Provider:  chat.ProviderOllama,
		Model:     "local",
	})
	if !errors.Is(err, chat.ErrEncryptedVault) {
		t.Fatalf("PrepareChat error = %v, want ErrEncryptedVault", err)
	}
	if app.session.chat != nil {
		t.Fatal("le service de chat a été créé pour un coffre chiffré")
	}
	if _, statErr := os.Stat(filepath.Join(app.session.service.Root(), ".notevault", "chat")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("état dérivé créé pour le coffre chiffré : %v", statErr)
	}
}

func TestAppChatRejectsNoteTraversalBeforeInitializingModels(t *testing.T) {
	app := setupAppForTest(t)
	_, err := app.PrepareChat(chat.PrepareRequest{
		NotePaths: []string{"notes/../../secret.md"},
		Question:  "Lis ce fichier",
		Provider:  chat.ProviderOllama,
		Model:     "local",
	})
	if err == nil {
		t.Fatal("PrepareChat a accepté un chemin traversant")
	}
	if app.session.chat != nil {
		t.Fatal("le service de chat a été initialisé avant la validation des notes")
	}
}

func TestAppChatSettingsAndSecretLifecycle(t *testing.T) {
	app := setupAppForTest(t)
	secrets := &fakeAppSecretStore{keys: make(map[chat.Provider]string)}
	app.secrets = secrets

	if err := app.UpdateChatPreferences(chat.ProviderOpenAI, " gpt-test "); err != nil {
		t.Fatalf("UpdateChatPreferences: %v", err)
	}
	if err := app.StoreChatAPIKey(chat.ProviderOpenAI, "first-secret"); err != nil {
		t.Fatalf("StoreChatAPIKey: %v", err)
	}
	if err := app.StoreChatAPIKey(chat.ProviderOpenAI, "replacement-secret"); err != nil {
		t.Fatalf("replace StoreChatAPIKey: %v", err)
	}
	settings, err := app.GetChatSettings()
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if settings.Provider != chat.ProviderOpenAI || settings.Models[chat.ProviderOpenAI] != "gpt-test" || !settings.KeyringAvailable {
		t.Fatalf("settings = %+v", settings)
	}
	if len(settings.ProvidersWithAPIKey) != 1 || settings.ProvidersWithAPIKey[0] != chat.ProviderOpenAI {
		t.Fatalf("providers with key = %+v", settings.ProvidersWithAPIKey)
	}
	raw, err := os.ReadFile(app.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "first-secret") || strings.Contains(string(raw), "replacement-secret") || strings.Contains(string(raw), "apiKey") {
		t.Fatal("une clé API apparaît dans app.json")
	}
	if err := app.DeleteChatAPIKey(chat.ProviderOpenAI); err != nil {
		t.Fatalf("DeleteChatAPIKey: %v", err)
	}
	if err := app.DeleteChatAPIKey(chat.ProviderOpenAI); err != nil {
		t.Fatalf("DeleteChatAPIKey idempotent: %v", err)
	}
	settings, err = app.GetChatSettings()
	if err != nil || len(settings.ProvidersWithAPIKey) != 0 {
		t.Fatalf("settings after delete = %+v, %v", settings, err)
	}
}

func TestAppChatSettingsReportUnavailableKeyring(t *testing.T) {
	app := setupAppForTest(t)
	app.secrets = &fakeAppSecretStore{availableErr: chat.ErrKeyringUnavailable}
	settings, err := app.GetChatSettings()
	if err != nil {
		t.Fatalf("GetChatSettings: %v", err)
	}
	if settings.KeyringAvailable || len(settings.ProvidersWithAPIKey) != 0 {
		t.Fatalf("settings = %+v", settings)
	}
	if err := app.StoreChatAPIKey(chat.ProviderMistral, "secret"); !errors.Is(err, chat.ErrKeyringUnavailable) {
		t.Fatalf("StoreChatAPIKey error = %v", err)
	}
}

func TestApplicationBackgroundIsOpaque(t *testing.T) {
	app := &App{}
	background := applicationOptions(app).BackgroundColour
	if background == nil {
		t.Fatal("BackgroundColour is nil")
	}
	if background.A != 255 {
		t.Fatalf("background alpha = %d, want 255", background.A)
	}
}

func TestApplicationLinuxDesktopIdentity(t *testing.T) {
	linuxOptions := applicationOptions(&App{}).Linux
	if linuxOptions == nil {
		t.Fatal("Linux options are nil")
	}
	if linuxOptions.ProgramName != "notevault" {
		t.Fatalf("ProgramName = %q, want %q", linuxOptions.ProgramName, "notevault")
	}
	if len(linuxOptions.Icon) == 0 {
		t.Fatal("Linux application icon is empty")
	}
	if linuxOptions.WebviewGpuPolicy != linux.WebviewGpuPolicyNever {
		t.Fatalf("WebviewGpuPolicy = %d, want disabled", linuxOptions.WebviewGpuPolicy)
	}
}
