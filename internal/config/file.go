package config

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"go.yaml.in/yaml/v3"
	"gopkg.in/ini.v1"
)

func readFile(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var fields map[string]any
	switch strings.ToLower(filepath.Ext(path)) {
	case ".toml":
		err = toml.Unmarshal(data, &fields)
	case ".yaml", ".yml":
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		err = decoder.Decode(&fields)
		if err == nil && !errors.Is(decoder.Decode(new(any)), io.EOF) {
			return nil, errors.New("expected one YAML document")
		}
	case ".json":
		err = json.Unmarshal(data, &fields)
	case ".ini":
		fields, err = readINI(data)
	case ".xml":
		fields, err = readXML(data)
	default:
		return nil, fmt.Errorf("unsupported configuration extension %q", filepath.Ext(path))
	}
	if err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, errors.New("expected a configuration object")
	}
	return fields, nil
}

func readINI(data []byte) (map[string]any, error) {
	file, err := ini.Load(data)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]any)
	for _, section := range file.Sections() {
		if section.Name() == ini.DEFAULT_SECTION {
			if len(section.Keys()) > 0 {
				return nil, errors.New("INI settings must be inside a named section")
			}
			continue
		}
		values := make(map[string]any)
		for _, key := range section.Keys() {
			values[key.Name()] = key.Value()
		}
		fields[section.Name()] = values
	}
	return fields, nil
}

func readXML(data []byte) (map[string]any, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root xmlConfig
	if err := decoder.Decode(&root); err != nil {
		return nil, err
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return root.fields, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.Comment:
		case xml.CharData:
			if strings.TrimSpace(string(t)) == "" {
				continue
			}
			return nil, errors.New("unexpected text after XML config root")
		default:
			return nil, errors.New("unexpected content after XML config root")
		}
	}
}

type xmlConfig struct {
	fields map[string]any
}

func (c *xmlConfig) UnmarshalXML(decoder *xml.Decoder, start xml.StartElement) error {
	if start.Name.Local != "config" {
		return errors.New("XML config requires a <config> root")
	}
	value, err := readXMLValue(decoder, start, 0)
	if err != nil {
		return err
	}
	if fields, ok := value.(map[string]any); ok {
		c.fields = fields
		return nil
	}
	return errors.New("XML config requires section elements")
}

func readXMLValue(decoder *xml.Decoder, start xml.StartElement, depth int) (any, error) {
	if start.Name.Space != "" || len(start.Attr) > 0 || depth > 2 {
		return nil, fmt.Errorf("invalid XML configuration element %q", start.Name.Local)
	}
	fields := make(map[string]any)
	var content strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			key := t.Name.Local
			if _, exists := fields[key]; exists {
				return nil, fmt.Errorf("duplicate XML config element %q", key)
			}
			value, err := readXMLValue(decoder, t, depth+1)
			if err != nil {
				return nil, err
			}
			fields[key] = value
		case xml.CharData:
			content.Write(t)
		case xml.EndElement:
			value := strings.TrimSpace(content.String())
			if depth == 2 {
				return value, nil
			}
			if value != "" {
				return nil, fmt.Errorf("unexpected XML text in %q", start.Name.Local)
			}
			return fields, nil
		}
	}
}
