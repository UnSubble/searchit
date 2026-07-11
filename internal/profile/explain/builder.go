package explain

import (
	"fmt"
	"strings"

	"github.com/unsubble/searchit/internal/profile"
	"github.com/unsubble/searchit/internal/profile/resolver"
	"github.com/unsubble/searchit/internal/profile/scan"
)

// Build constructs the ExplanationModel for a profile after resolving dependencies.
func Build(store profile.Store, rootName string) (*ExplanationModel, error) {
	// 1. Resolve the dependency graph (topo-sorted order: dependencies first, target last).
	res := resolver.New(store)
	resolved, err := res.Resolve([]string{rootName})
	if err != nil {
		return nil, err
	}

	// 2. Perform two-stage validation on each profile in the resolved chain.
	for _, p := range resolved {
		// Generic validation
		if err := profile.Validate(p); err != nil {
			return nil, fmt.Errorf("profile %s validation failed: %w", p.Name, err)
		}
		// Tool-specific validation
		if v := profile.GetValidator(p.Tool); v != nil {
			if err := v.Validate(p); err != nil {
				return nil, fmt.Errorf("profile %s validator failed: %w", p.Name, err)
			}
		}
	}

	// 3. Initialize default configuration values.
	finalConfig := map[string]string{
		"Threads":   "32",
		"Recursive": "false",
		"Strategy":  "bfs",
		"Max Depth": "3",
		"Output":    "text",
		"Timeout":   "10s",
		"Delay":     "0",
		"Rate":      "unlimited",
	}

	listFields := map[string][]string{
		"Recurse On":      {"200", "301", "302", "403"},
		"Exclude Status":  {"404"},
		"Include Headers": {},
		"Exclude Headers": {},
	}

	// Track override history for each of the 8 single-value fields.
	overridesMap := map[string][]FieldOverride{
		"Threads":   {},
		"Recursive": {},
		"Strategy":  {},
		"Max Depth": {},
		"Output":    {},
		"Timeout":   {},
		"Delay":     {},
		"Rate":      {},
	}

	dependsChain := make([]string, len(resolved))
	for i, p := range resolved {
		dependsChain[i] = p.Name
		shortName := strings.Split(p.Name, "/")[len(strings.Split(p.Name, "/"))-1]

		var o scan.Overlay
		if err := p.Decode(&o); err != nil {
			return nil, fmt.Errorf("decode profile %s overlay: %w", p.Name, err)
		}

		// Threads
		if o.Threads != nil {
			val := fmt.Sprintf("%d", *o.Threads)
			overridesMap["Threads"] = append(overridesMap["Threads"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Threads"] = val
		}
		// Recursive
		if o.Recursive != nil {
			val := fmt.Sprintf("%t", *o.Recursive)
			overridesMap["Recursive"] = append(overridesMap["Recursive"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Recursive"] = val
		}
		// Strategy
		if o.Strategy != nil {
			val := *o.Strategy
			overridesMap["Strategy"] = append(overridesMap["Strategy"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Strategy"] = val
		}
		// Max Depth
		if o.MaxDepth != nil {
			val := fmt.Sprintf("%d", *o.MaxDepth)
			overridesMap["Max Depth"] = append(overridesMap["Max Depth"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Max Depth"] = val
		}
		// Output
		if o.Output != nil {
			val := *o.Output
			overridesMap["Output"] = append(overridesMap["Output"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Output"] = val
		}
		// Timeout
		if o.Timeout != nil {
			val := o.Timeout.String()
			overridesMap["Timeout"] = append(overridesMap["Timeout"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Timeout"] = val
		}
		// Delay
		if o.Delay != nil {
			var val string
			if *o.Delay == 0 {
				val = "0"
			} else {
				val = o.Delay.String()
			}
			overridesMap["Delay"] = append(overridesMap["Delay"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Delay"] = val
		}
		// Rate
		if o.Rate != nil {
			var val string
			if *o.Rate <= 0 {
				val = "unlimited"
			} else {
				val = fmt.Sprintf("%g", *o.Rate)
			}
			overridesMap["Rate"] = append(overridesMap["Rate"], FieldOverride{ProfileName: shortName, Value: val})
			finalConfig["Rate"] = val
		}

		// Recurse On
		if o.RecurseOn != nil {
			items := strings.Split(*o.RecurseOn, ",")
			var trimmed []string
			for _, it := range items {
				trimmed = append(trimmed, strings.TrimSpace(it))
			}
			listFields["Recurse On"] = trimmed
		}
		// Exclude Status
		if o.ExcludeStatus != nil {
			items := strings.Split(*o.ExcludeStatus, ",")
			var trimmed []string
			for _, it := range items {
				trimmed = append(trimmed, strings.TrimSpace(it))
			}
			listFields["Exclude Status"] = trimmed
		}
		// Include Headers
		if o.IncludeHeaders != nil {
			listFields["Include Headers"] = *o.IncludeHeaders
		}
		// Exclude Headers
		if o.ExcludeHeaders != nil {
			listFields["Exclude Headers"] = *o.ExcludeHeaders
		}
	}

	// Filter and convert overridesMap to the ordered FieldOverrides slice.
	// Only include fields that were overridden by at least one profile.
	var fieldOverrides []FieldExplanation
	fieldOrder := []string{"Threads", "Recursive", "Strategy", "Max Depth", "Output", "Timeout", "Delay", "Rate"}
	for _, name := range fieldOrder {
		if history := overridesMap[name]; len(history) > 0 {
			fieldOverrides = append(fieldOverrides, FieldExplanation{
				Name:      name,
				Final:     finalConfig[name],
				Overrides: history,
			})
		}
	}

	// Find the target profile metadata from the last element of resolved chain.
	target := resolved[len(resolved)-1]

	return &ExplanationModel{
		TargetProfile:  target,
		DependsChain:   dependsChain,
		FinalConfig:    finalConfig,
		ListFields:     listFields,
		FieldOverrides: fieldOverrides,
	}, nil
}
