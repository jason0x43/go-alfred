package alfred

import (
	"log"
	"path"
	"os/user"
	"encoding/xml"
	"github.com/jason0x43/go-plist"
)

//
// Public API
//

var MaxResults = 9
var cacheRoot string
var dataRoot string

type Item struct {
	XMLName      xml.Name `xml:"item"`
	Uid          string   `xml:"uid,attr,omitempty"`
	Arg          string   `xml:"arg,omitempty"`
	Title        string   `xml:"title"`
	Subtitle     string   `xml:"subtitle,omitempty"`
	Icon         string   `xml:"icon,omitempty"`
	Valid        bool     `xml:"valid,attr"`
	Autocomplete string   `xml:"autocomplete,attr,omitempty"`
}

func ToXML(results []Item) string {
	newxml := "<items>"

	for i := 0; i < len(results); i++ {
		data, err := xml.Marshal(results[i])
		if err != nil {
			log.Fatalf("ToXML Error: %v\n", err)
		}
		newxml += string(data)
	}

	newxml += "</items>"
	return newxml
}

type Workflow struct {
	bundleId string
}

func OpenWorkflow(workflowDir string) (*Workflow, error) {
	pl, err := plist.UnmarshalFile("info.plist")
	if err != nil {
		log.Println("alfred: Error opening plist:", err)
	}

	plData := pl.Root.(plist.Dict)
	bundleId := plData["bundleid"].(string)

	w := Workflow{bundleId: bundleId}
	return &w, nil
}

func (w *Workflow) CacheDir() string {
	return path.Join(cacheRoot, w.bundleId)
}

func (w *Workflow) DataDir() string {
	return path.Join(dataRoot, w.bundleId)
}

func (w *Workflow) BundleId() string {
	return w.bundleId
}

//
// Internal
//

func init() {
	u, err := user.Current()
	if err != nil {
		log.Fatal("Couldn't access current user")
	}
	home := u.HomeDir

	cacheRoot = path.Join(home, "Library", "Caches", "com.runningwithcrayons.Alfred-2",
		"Workflow Data");
	dataRoot = path.Join(home, "Library", "Application Support", "Alfred 2",
		"Workflow Data");
}
