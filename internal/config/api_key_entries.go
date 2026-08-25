package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// AccessAPIKeyEntry stores a client API key plus optional per-client settings.
// APIKey is used to authenticate requests, Name is UI metadata, and ForceModel
// rewrites model-bearing requests after authentication.
type AccessAPIKeyEntry struct {
	APIKey     string `yaml:"api-key" json:"api-key"`
	Name       string `yaml:"name,omitempty" json:"name,omitempty"`
	ForceModel string `yaml:"force-model,omitempty" json:"force-model,omitempty"`
}

func normalizeAccessAPIKeyEntry(entry AccessAPIKeyEntry) AccessAPIKeyEntry {
	return AccessAPIKeyEntry{
		APIKey:     strings.TrimSpace(entry.APIKey),
		Name:       strings.TrimSpace(entry.Name),
		ForceModel: strings.TrimSpace(entry.ForceModel),
	}
}

func normalizeAccessAPIKeyEntries(entries []AccessAPIKeyEntry) ([]AccessAPIKeyEntry, []string) {
	normalized := make([]AccessAPIKeyEntry, 0, len(entries))
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		entry = normalizeAccessAPIKeyEntry(entry)
		if entry.APIKey == "" {
			continue
		}
		normalized = append(normalized, entry)
		keys = append(keys, entry.APIKey)
	}
	return normalized, keys
}

// NormalizeAccessAPIKeyEntriesForManagement normalizes management API input and
// returns the runtime key list that must stay aligned with the entries.
func NormalizeAccessAPIKeyEntriesForManagement(entries []AccessAPIKeyEntry) ([]AccessAPIKeyEntry, []string) {
	return normalizeAccessAPIKeyEntries(entries)
}

// AccessAPIKeyEntriesForKeys returns entries aligned with keys. Existing names
// are preserved first by same index, then by same key value.
func AccessAPIKeyEntriesForKeys(keys []string, existing []AccessAPIKeyEntry) []AccessAPIKeyEntry {
	if len(keys) == 0 {
		return nil
	}

	namesByKey := make(map[string]string, len(existing))
	forceModelsByKey := make(map[string]string, len(existing))
	for _, entry := range existing {
		entry = normalizeAccessAPIKeyEntry(entry)
		if entry.APIKey == "" {
			continue
		}
		if entry.Name != "" {
			if _, ok := namesByKey[entry.APIKey]; !ok {
				namesByKey[entry.APIKey] = entry.Name
			}
		}
		if entry.ForceModel != "" {
			if _, ok := forceModelsByKey[entry.APIKey]; !ok {
				forceModelsByKey[entry.APIKey] = entry.ForceModel
			}
		}
	}

	out := make([]AccessAPIKeyEntry, 0, len(keys))
	for i, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		name := ""
		forceModel := ""
		if i < len(existing) {
			candidate := normalizeAccessAPIKeyEntry(existing[i])
			if candidate.APIKey == key {
				name = candidate.Name
				forceModel = candidate.ForceModel
			}
		}
		if name == "" {
			name = namesByKey[key]
		}
		if forceModel == "" {
			forceModel = forceModelsByKey[key]
		}
		out = append(out, AccessAPIKeyEntry{APIKey: key, Name: name, ForceModel: forceModel})
	}
	return out
}

func parseAccessAPIKeyEntriesNode(node *yaml.Node) ([]AccessAPIKeyEntry, []string, error) {
	if node == nil {
		return nil, nil, nil
	}
	if node.Kind == yaml.AliasNode && node.Alias != nil {
		return parseAccessAPIKeyEntriesNode(node.Alias)
	}
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil, nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, nil, fmt.Errorf("api-keys must be a sequence")
	}

	entries := make([]AccessAPIKeyEntry, 0, len(node.Content))
	for i, item := range node.Content {
		if item == nil {
			continue
		}
		switch item.Kind {
		case yaml.ScalarNode:
			key := strings.TrimSpace(item.Value)
			if key != "" {
				entries = append(entries, AccessAPIKeyEntry{APIKey: key})
			}
		case yaml.MappingNode:
			var entry AccessAPIKeyEntry
			if err := item.Decode(&entry); err != nil {
				return nil, nil, fmt.Errorf("api-keys[%d]: %w", i, err)
			}
			entry = normalizeAccessAPIKeyEntry(entry)
			if entry.APIKey != "" {
				entries = append(entries, entry)
			}
		default:
			return nil, nil, fmt.Errorf("api-keys[%d] must be a string or mapping", i)
		}
	}

	normalized, keys := normalizeAccessAPIKeyEntries(entries)
	return normalized, keys, nil
}

// decodeConfigYAML decodes a config document while accepting both legacy string
// api-keys entries and object entries carrying display metadata or force-model.
// Config.APIKeys remains the runtime []string shape; APIKeyEntries keeps the
// aligned metadata used by management and request policy code.
func decodeConfigYAML(data []byte, cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config destination is nil")
	}

	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if root.Kind == 0 {
		cfg.NormalizeAccessAPIKeyEntries()
		return nil
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 || root.Content[0] == nil || root.Content[0].Kind != yaml.MappingNode {
		if err := root.Decode(cfg); err != nil {
			return err
		}
		cfg.NormalizeAccessAPIKeyEntries()
		return nil
	}

	mapping := root.Content[0]
	apiKeys := apiKeysNode(mapping)
	if apiKeys == nil {
		if err := root.Decode(cfg); err != nil {
			return err
		}
		cfg.NormalizeAccessAPIKeyEntries()
		return nil
	}

	entries, keys, errParse := parseAccessAPIKeyEntriesNode(apiKeys)
	if errParse != nil {
		return errParse
	}
	replaceAPIKeysNode(mapping, keys, nil)
	if errDecode := root.Decode(cfg); errDecode != nil {
		return errDecode
	}
	cfg.APIKeyEntries = entries
	cfg.NormalizeAccessAPIKeyEntries()
	return nil
}

func accessAPIKeyEntriesNeedObjectYAML(entries []AccessAPIKeyEntry) bool {
	for _, entry := range entries {
		if strings.TrimSpace(entry.Name) != "" || strings.TrimSpace(entry.ForceModel) != "" {
			return true
		}
	}
	return false
}

func buildAPIKeysYAMLNode(keys []string, entries []AccessAPIKeyEntry) *yaml.Node {
	entries = AccessAPIKeyEntriesForKeys(keys, entries)
	node := &yaml.Node{Kind: yaml.SequenceNode}
	if len(entries) == 0 {
		return node
	}

	if !accessAPIKeyEntriesNeedObjectYAML(entries) {
		for _, entry := range entries {
			node.Content = append(node.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.APIKey})
		}
		return node
	}

	for _, entry := range entries {
		item := &yaml.Node{Kind: yaml.MappingNode}
		item.Content = append(item.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "api-key"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.APIKey},
		)
		if entry.Name != "" {
			item.Content = append(item.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.Name},
			)
		}
		if entry.ForceModel != "" {
			item.Content = append(item.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "force-model"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: entry.ForceModel},
			)
		}
		node.Content = append(node.Content, item)
	}
	return node
}

func cloneYAMLNode(node *yaml.Node) *yaml.Node {
	if node == nil {
		return nil
	}
	cloned := *node
	if len(node.Content) > 0 {
		cloned.Content = make([]*yaml.Node, len(node.Content))
		for i := range node.Content {
			cloned.Content[i] = cloneYAMLNode(node.Content[i])
		}
	}
	return &cloned
}

func apiKeysNode(root *yaml.Node) *yaml.Node {
	if root == nil || root.Kind != yaml.MappingNode {
		return nil
	}
	idx := findMapKeyIndex(root, "api-keys")
	if idx < 0 || idx+1 >= len(root.Content) {
		return nil
	}
	return root.Content[idx+1]
}

func replaceAPIKeysNode(root *yaml.Node, keys []string, entries []AccessAPIKeyEntry) {
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}
	next := buildAPIKeysYAMLNode(keys, entries)
	idx := findMapKeyIndex(root, "api-keys")
	if idx < 0 {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "api-keys"},
			next,
		)
		return
	}
	if idx+1 < len(root.Content) {
		root.Content[idx+1] = next
	}
}

// NormalizeAccessAPIKeyEntries keeps APIKeyEntries aligned with APIKeys.
func (cfg *Config) NormalizeAccessAPIKeyEntries() {
	if cfg == nil {
		return
	}
	cfg.APIKeyEntries = AccessAPIKeyEntriesForKeys(cfg.APIKeys, cfg.APIKeyEntries)
}
