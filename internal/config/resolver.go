package config

// ResolveScanConfig resolves the baseline config for scan mode by composing:
// 1. Built-in defaults (config.Default())
// 2. Global config file (scan: overlay)
// 3. Profiles in deterministic left-to-right order
func ResolveScanConfig(configFile string, profileOverlays []ScanOverlay) (Config, error) {
	cfg := Default()

	f, _, err := LoadFile(configFile)
	if err != nil {
		return Config{}, err
	}

	if f != nil && f.Scan != nil {
		ApplyScanOverlay(&cfg, *f.Scan)
	}

	for _, p := range profileOverlays {
		ApplyScanOverlay(&cfg, p)
	}

	return cfg, nil
}

// ResolveFuzzConfig resolves the baseline config for fuzz mode by composing:
// 1. Built-in defaults (config.Default())
// 2. Global config file (fuzz: overlay)
// 3. Profiles in deterministic left-to-right order
func ResolveFuzzConfig(configFile string, profileOverlays []FuzzOverlay) (Config, error) {
	cfg := Default()

	f, _, err := LoadFile(configFile)
	if err != nil {
		return Config{}, err
	}

	if f != nil && f.Fuzz != nil {
		ApplyFuzzOverlay(&cfg, *f.Fuzz)
	}

	for _, p := range profileOverlays {
		ApplyFuzzOverlay(&cfg, p)
	}

	return cfg, nil
}
