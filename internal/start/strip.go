package start

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// StripPorts parses a docker-compose YAML document and removes all `ports:`
// entries from services. If keepPorts is non-empty, services whose names match
// entries in keepPorts retain their ports. All other YAML content (variables,
// anchors, comments, structure) is preserved via the yaml.v3 Node API.
func StripPorts(data []byte, keepPorts []string) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}

	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return data, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return data, nil
	}

	keepSet := make(map[string]bool, len(keepPorts))
	for _, s := range keepPorts {
		keepSet[s] = true
	}

	for i := 0; i < len(root.Content)-1; i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]

		if keyNode.Value != "services" || valNode.Kind != yaml.MappingNode {
			continue
		}

		for j := 0; j < len(valNode.Content)-1; j += 2 {
			svcName := valNode.Content[j].Value
			svcNode := valNode.Content[j+1]

			if keepSet[svcName] {
				continue
			}

			if svcNode.Kind != yaml.MappingNode {
				continue
			}

			stripPortsFromService(svcNode)
		}
	}

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return nil, fmt.Errorf("marshaling YAML: %w", err)
	}
	return out, nil
}

func stripPortsFromService(svc *yaml.Node) {
	filtered := make([]*yaml.Node, 0, len(svc.Content))
	for i := 0; i < len(svc.Content)-1; i += 2 {
		if svc.Content[i].Value == "ports" {
			continue
		}
		filtered = append(filtered, svc.Content[i], svc.Content[i+1])
	}
	svc.Content = filtered
}
