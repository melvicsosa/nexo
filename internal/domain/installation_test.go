package domain

import (
	"testing"
	"time"
)

func TestTargetValidate(t *testing.T) {
	tests := []struct {
		name    string
		target  Target
		wantErr bool
	}{
		{"global without path", Target{Scope: ScopeGlobal}, false},
		{"global with path", Target{Scope: ScopeGlobal, ProjectPath: "/x"}, true},
		{"project with path", Target{Scope: ScopeProject, ProjectPath: "/x"}, false},
		{"project without path", Target{Scope: ScopeProject}, true},
		{"unknown scope", Target{Scope: "workspace"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.target.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestInstallationValidate(t *testing.T) {
	valid := Installation{
		Asset:       ID{Source: "local", Name: "x"},
		Hash:        "abc",
		Provider:    "claude-code",
		Target:      Target{Scope: ScopeGlobal},
		Strategy:    StrategyMaterialize,
		Source:      SourceLibrary,
		InstalledAt: time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC),
	}
	tests := []struct {
		name    string
		mutate  func(Installation) Installation
		wantErr bool
	}{
		{"valid", func(in Installation) Installation { return in }, false},
		{"missing hash", func(in Installation) Installation { in.Hash = ""; return in }, true},
		{"missing provider", func(in Installation) Installation { in.Provider = ""; return in }, true},
		{"bad target", func(in Installation) Installation { in.Target = Target{Scope: ScopeProject}; return in }, true},
		{"bad strategy", func(in Installation) Installation { in.Strategy = "yolo"; return in }, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.mutate(valid).Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
