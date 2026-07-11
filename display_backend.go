package main

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const displayBackendOverrideEnv = "NOTEAULT_GDK_BACKEND"

// configureDisplayBackend sélectionne XWayland pour WebKitGTK sous Hyprland.
// Avec le backend GTK Wayland, WebKitGTK 4.1 peut publier un buffer dont la
// hauteur est tronquée après un changement de focus, alors que GTK et le DOM
// conservent les bonnes dimensions. L'override permet de retester Wayland
// natif lorsque le bug sera corrigé en amont.
func configureDisplayBackend() error {
	backend := preferredGDKBackend(
		runtime.GOOS,
		os.Getenv("HYPRLAND_INSTANCE_SIGNATURE"),
		os.Getenv("XDG_CURRENT_DESKTOP"),
		os.Getenv(displayBackendOverrideEnv),
		os.Getenv("GDK_BACKEND"),
	)
	if backend == "" || backend == os.Getenv("GDK_BACKEND") {
		return nil
	}
	if err := os.Setenv("GDK_BACKEND", backend); err != nil {
		return fmt.Errorf("configurer le backend d'affichage GTK : %w", err)
	}
	return nil
}

func preferredGDKBackend(goos, hyprlandSignature, desktop, override, current string) string {
	if override != "" {
		return override
	}
	if goos == "linux" && (hyprlandSignature != "" || strings.EqualFold(desktop, "Hyprland")) {
		return "x11"
	}
	return current
}
