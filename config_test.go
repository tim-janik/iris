package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSiteConfigRejectsNegativeLengths(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "_siteconfig.toml")
	content := `
title = "Test Site"
teaser_len = -50
desc_len = -1
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg := loadSiteConfig(dir, path)
	def := defaultSiteConfig()
	if cfg.TeaserLen != def.TeaserLen {
		t.Errorf("TeaserLen = %d, want default %d", cfg.TeaserLen, def.TeaserLen)
	}
	if cfg.DescLen != def.DescLen {
		t.Errorf("DescLen = %d, want default %d", cfg.DescLen, def.DescLen)
	}
	if cfg.Title != "Test Site" {
		t.Errorf("Title = %q, want valid settings to be kept", cfg.Title)
	}
}
