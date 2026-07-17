package assayparser

import "os"

// import "strings"
import "encoding/json"
import "gopkg.in/yaml.v3"
import "errors"

var ioError error = errors.New("Error in the IO process. Check inputs!")
var DefaultPathJSON string = "./out/outputJSON.json"

func ConvertJson(a ValidAssay) ([]byte, error) {
	result, err := json.Marshal(a)
	if err != nil {
		return nil, ioError
	}
	return result, nil
}

func ExportJson(path string, js []byte) error {
	if path == "" || js == nil {
		return ioError
	}
	err := os.WriteFile(path, js, 0644)
	if err != nil {
		return ioError
	}
	return nil
}

func ConvertYaml(a ValidAssay) ([]byte, error) {
	result, err := yaml.Marshal(a)
	if err != nil {
		return nil, ioError
	}
	return result, nil
}

func ExportYaml(path string, ya []byte) error {
	if path == "" || ya == nil {
		return ioError
	}
	err := os.WriteFile(path, ya, 0644)
	if err != nil {
		return ioError
	}
	return nil
}

func ImportFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, ioError
	}
	return data, nil
}

func UnwindJson(in []byte) (ValidAssay, error) {
	var a ValidAssay
	err := json.Unmarshal(in, &a)
	if err != nil {
		return ValidAssay{}, ioError
	}
	return a, nil
}

func UnwindYaml(in []byte) (ValidAssay, error) {
	var a ValidAssay
	err := yaml.Unmarshal(in, &a)
	if err != nil {
		return ValidAssay{}, ioError
	}
	return a, nil
}
