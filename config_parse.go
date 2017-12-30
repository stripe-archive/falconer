package falconer

import (
	"io/ioutil"
	"os"

	"gopkg.in/yaml.v2"
)

// ReadConfig reads the configuration from `path` and returns a filled-in config.
func ReadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()

	yamlContents, err := ioutil.ReadAll(f)
	if err != nil {
		return Config{}, err
	}

	config := Config{}
	err = yaml.Unmarshal(yamlContents, &config)
	if err != nil {
		return config, err
	}

	return config, nil
}
