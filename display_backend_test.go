package main

import "testing"

func TestPreferredGDKBackend(t *testing.T) {
	tests := []struct {
		name      string
		goos      string
		signature string
		desktop   string
		override  string
		current   string
		want      string
	}{
		{
			name:      "Hyprland signature uses XWayland",
			goos:      "linux",
			signature: "instance",
			current:   "wayland,x11,*",
			want:      "x11",
		},
		{
			name:    "Hyprland desktop uses XWayland",
			goos:    "linux",
			desktop: "Hyprland",
			want:    "x11",
		},
		{
			name:      "explicit override restores native Wayland",
			goos:      "linux",
			signature: "instance",
			override:  "wayland",
			current:   "x11",
			want:      "wayland",
		},
		{
			name:    "other Linux desktop keeps current backend",
			goos:    "linux",
			desktop: "GNOME",
			current: "wayland,x11,*",
			want:    "wayland,x11,*",
		},
		{
			name:    "other platform keeps current backend",
			goos:    "darwin",
			desktop: "Hyprland",
			current: "quartz",
			want:    "quartz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := preferredGDKBackend(
				tt.goos,
				tt.signature,
				tt.desktop,
				tt.override,
				tt.current,
			)
			if got != tt.want {
				t.Fatalf("preferredGDKBackend() = %q, want %q", got, tt.want)
			}
		})
	}
}
