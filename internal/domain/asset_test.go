package domain

import "testing"

func TestParseID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ID
		wantErr bool
	}{
		{"bare name gets default source", "wordpress-review", ID{Source: "local", Name: "wordpress-review"}, false},
		{"explicit source", "company/wordpress-review", ID{Source: "company", Name: "wordpress-review"}, false},
		{"empty string", "", ID{}, true},
		{"empty name", "company/", ID{}, true},
		{"empty source", "/name", ID{}, true},
		{"too many segments", "a/b/c", ID{}, true},
		{"whitespace in name", "company/my skill", ID{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseID(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseID(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("ParseID(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestIDString(t *testing.T) {
	id := ID{Source: "company", Name: "review"}
	if got := id.String(); got != "company/review" {
		t.Errorf("String() = %q, want %q", got, "company/review")
	}
}

func TestAssetValidate(t *testing.T) {
	valid := Asset{
		ID:   ID{Source: "local", Name: "x"},
		Type: TypeSkill,
		Hash: "abc123",
	}
	tests := []struct {
		name    string
		mutate  func(Asset) Asset
		wantErr bool
	}{
		{"valid unversioned asset", func(a Asset) Asset { return a }, false},
		{"valid versioned asset", func(a Asset) Asset { a.Version = "1.2.0"; return a }, false},
		{"unknown type", func(a Asset) Asset { a.Type = "widget"; return a }, true},
		{"missing hash", func(a Asset) Asset { a.Hash = ""; return a }, true},
		{"invalid id", func(a Asset) Asset { a.ID.Name = ""; return a }, true},
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

func TestAssetUnversioned(t *testing.T) {
	if !(Asset{}).Unversioned() {
		t.Error("empty version should be unversioned")
	}
	if (Asset{Version: "1.0.0"}).Unversioned() {
		t.Error("versioned asset reported as unversioned")
	}
}
