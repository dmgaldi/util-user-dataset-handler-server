package config

import (
	"gopkg.in/yaml.v3"
	"os"
)

type FileOptions interface {
	Commands() Command
	FileTypes() []string
	ServiceName() string
}

func ParseFileOptions(path string) (FileOptions, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	out := new(fileOptions)
	dec := yaml.NewDecoder(file)
	if err := dec.Decode(out); err != nil {
		return nil, err
	}

	out.Extensions = appendDefaultFileTypes(out.Extensions)

	return out, nil
}

func appendDefaultFileTypes(types []string) []string {
	return append(types,
		".tar.gz",
		".tgz",
		".zip")
}

type fileOptions struct {
	Command    Command  `yaml:"command"`
	Extensions []string `yaml:"fileTypes"`
	SvcName    string   `yaml:"serviceName"`
}

func (f *fileOptions) Commands() Command {
	return f.Command
}

func (f *fileOptions) FileTypes() []string {
	return f.Extensions
}

func (f *fileOptions) ServiceName() string {
	return f.SvcName
}
