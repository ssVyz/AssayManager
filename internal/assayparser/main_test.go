package assayparser

//import "fmt"
//import "os"
// import "strings"
//import "encoding/json"
//import "gopkg.in/yaml.v3"
//import "testing"






/*

func main() {

	h := ap.MkHeader("Test", "v1", "You what")
	o := ap.MkOligos()
	t := ap.MkTargets()

	w := ap.WrapAssay(h, o, t)

	fmt.Println("structs done")
	fmt.Println(w.Header.Name, w.Header.Version, w.Header.Author)

	data, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		panic(err)
	}

	err2 := os.WriteFile("test_output.json", data, 0644)
	if err2 != nil {
		panic(err2)
	}

	fmt.Println("json file done")

	yam, err3 := yaml.Marshal(w)
	if err3 != nil {
		panic(err3)
	}

	err4 := os.WriteFile("test_output_yam.yaml", yam, 0644)
	if err4 != nil {
		panic(err4)
	}


}
	*/
