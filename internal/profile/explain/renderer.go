package explain

import (
	"fmt"
	"strings"
)

// RenderText formats the ExplanationModel into a clean human-readable ASCII layout.
func RenderText(model *ExplanationModel) string {
	var sb strings.Builder

	// 1. Profile metadata block
	sb.WriteString("Profile\n\n")
	sb.WriteString("    " + model.TargetProfile.Name + "\n\n")

	if model.TargetProfile.Description != "" {
		sb.WriteString("Description\n\n")
		sb.WriteString("    " + model.TargetProfile.Description + "\n\n")
	}

	if model.TargetProfile.Author != "" {
		sb.WriteString("Author\n\n")
		sb.WriteString("    " + model.TargetProfile.Author + "\n\n")
	}

	if model.TargetProfile.Homepage != "" {
		sb.WriteString("Homepage\n\n")
		sb.WriteString("    " + model.TargetProfile.Homepage + "\n\n")
	}

	if model.TargetProfile.License != "" {
		sb.WriteString("License\n\n")
		sb.WriteString("    " + model.TargetProfile.License + "\n\n")
	}

	if len(model.TargetProfile.Tags) > 0 {
		sb.WriteString("Tags\n\n")
		for _, tag := range model.TargetProfile.Tags {
			sb.WriteString("    " + tag + "\n")
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Experimental\n\n")
	sb.WriteString(fmt.Sprintf("    %t\n\n", model.TargetProfile.Experimental))

	if model.TargetProfile.Created != "" {
		sb.WriteString("Created\n\n")
		sb.WriteString("    " + model.TargetProfile.Created + "\n\n")
	}

	if model.TargetProfile.Updated != "" {
		sb.WriteString("Updated\n\n")
		sb.WriteString("    " + model.TargetProfile.Updated + "\n\n")
	}

	// 2. Dependency chain block
	sb.WriteString("Dependency Chain\n\n")
	for i, name := range model.DependsChain {
		sb.WriteString("    " + name + "\n")
		if i < len(model.DependsChain)-1 {
			sb.WriteString("        \u2193\n") // Downwards arrow
		}
	}
	sb.WriteString("\n")

	// 3. Applied order block
	sb.WriteString("Applied Order\n\n")
	for i, name := range model.DependsChain {
		sb.WriteString(fmt.Sprintf("    %d. %s\n", i+1, name))
	}
	sb.WriteString("\n")

	// 4. Final configuration block
	sb.WriteString("Final Configuration\n\n")
	keys := []string{"Threads", "Recursive", "Strategy", "Max Depth", "Output", "Timeout", "Delay", "Rate"}
	for _, key := range keys {
		val := model.FinalConfig[key]
		sb.WriteString(fmt.Sprintf("%-22s%s\n", key+":", val))
	}
	sb.WriteString("\n")

	listKeys := []string{"Recurse On", "Exclude Status", "Include Headers", "Exclude Headers"}
	for _, key := range listKeys {
		items := model.ListFields[key]
		if len(items) == 0 {
			continue
		}
		sb.WriteString(key + ":\n\n")
		for _, item := range items {
			sb.WriteString("    " + item + "\n")
		}
		sb.WriteString("\n")
	}

	// 5. Override information block
	if len(model.FieldOverrides) > 0 {
		for _, expl := range model.FieldOverrides {
			sb.WriteString(expl.Name + "\n\n")
			for _, ovr := range expl.Overrides {
				prefix := "    " + ovr.ProfileName
				sb.WriteString(fmt.Sprintf("%-18s%s\n\n", prefix, ovr.Value))
			}
			sb.WriteString("Final\n\n")
			sb.WriteString("    " + expl.Final + "\n\n")
		}
	}

	return strings.TrimSuffix(sb.String(), "\n")
}
