package alfred

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strings"

	"github.com/jason0x43/go-plist"
)

//
// Public API
//

const (
	LINE       = "––––––––––––––––––––––––––––––––––––––––––––––"
	INVALID    = "no"
	SEPARATOR  = " »"
	TERMINATOR = "⁣"
)

const (
	FILTER_MENU = 0x01
)

var MaxResults = 9

type Results struct {
	items []Item
}

type Filter interface {
	Keyword() string
	MenuItem() Item
	Items(prefix, query string) ([]Item, error)
}

type Action interface {
	Keyword() string
	Do(query string) (string, error)
}

type Item struct {
	Uid           string
	Arg           string
	Title         string
	Subtitle      string
	SubtitleAll   string
	SubtitleShift string
	SubtitleAlt   string
	SubtitleCmd   string
	SubtitleCtrl  string
	SubtitleFn    string
	Icon          string
	Valid         string
	Autocomplete  string
}

type XmlItem struct {
	XMLName      xml.Name      `xml:"item"`
	Uid          string        `xml:"uid,attr,omitempty"`
	Title        string        `xml:"title"`
	Subtitles    []XmlSubtitle `xml:"subtitle,omitempty"`
	Icon         string        `xml:"icon,omitempty"`
	Arg          string        `xml:"arg,attr,omitempty"`
	Valid        string        `xml:"valid,attr,omitempty"`
	Autocomplete string        `xml:"autocomplete,attr,omitempty"`
}

type XmlSubtitle struct {
	Mod   string `xml:"mod,attr,omitempty"`
	Value string `xml:",chardata"`
}

func (w *Workflow) Run(filters []Filter, actions []Action) {
	var op string
	var kind string
	var query string
	var err error

	log.Printf("args: %#v\n", os.Args)

	if len(os.Args) > 1 {
		op = os.Args[1]
	}

	if len(os.Args) > 3 {
		query = os.Args[3]
		kind = os.Args[2]
	} else if len(os.Args) > 2 {
		parts := strings.SplitN(os.Args[2], " ", 2)
		kind = parts[0]

		if len(parts) > 1 {
			query = parts[1]
		}
	}

	log.Printf("op: '%s'", op)
	log.Printf("kind: '%s'", kind)
	log.Printf("query: '%s'", query)

	switch op {
	case "tell":
		var err error
		var items []Item
		commands := findFilter(kind, filters)

		if len(commands) > 0 {
			var cmdItems []Item
			for _, c := range commands {
				cmdItems, err = c.Items(kind+" ", query)
				if err == nil {
					for _, i := range cmdItems {
						items = append(items, i)
					}
				}
			}
		} else {
			for _, c := range filters {
				if strings.HasPrefix(c.Keyword(), kind) {
					item := c.MenuItem()
					items = append(items, item)
				}
			}
		}

		if err != nil {
			log.Printf("Error: %s", err)
			items = append(items, Item{Title: fmt.Sprintf("Error: %s", err)})
		}

		SendToAlfred(items)

	case "do":
		var output string
		command := findAction(kind, actions)

		if command == nil {
			err = fmt.Errorf("Unknown command '%s'", kind)
		} else {
			output, err = command.Do(query)
		}

		if err != nil {
			output = fmt.Sprintf("Error: %s", err)
		}

		if output != "" {
			fmt.Println(output)
		}

	default:
		fmt.Printf("Invalid operation: '%s'\n", op)
	}
}

func SplitQuery(query string) []string {
	return strings.Split(query, SEPARATOR)
}

func SplitAndTrimQuery(query string) []string {
	parts := SplitQuery(query)
	for i, part := range parts {
		parts[i] = strings.TrimSpace(part)
	}
	return parts
}

func InsertItem(items []Item, item Item, index int) []Item {
	items = append(items, item)
	copy(items[index+1:], items[index:])
	items[index] = item
	return items
}

func SendToAlfred(items []Item) {
	fmt.Println(ToXML(items))
}

func ToXML(items []Item) string {
	newxml := "<?xml version=\"1.0\"?><items>"

	for _, item := range items {
		xmlItem := XmlItem{
			Uid:          item.Uid,
			Arg:          item.Arg,
			Title:        item.Title,
			Icon:         item.Icon,
			Valid:        item.Valid,
			Autocomplete: item.Autocomplete,
		}

		getSubtitle := func(subtitle string) string {
			if subtitle != "" {
				return subtitle
			}
			return item.SubtitleAll
		}

		addSubtitle := func(subtitle, mod string) {
			if st := getSubtitle(subtitle); st != "" {
				xmlItem.Subtitles = append(xmlItem.Subtitles, XmlSubtitle{mod, st})
			}
		}

		addSubtitle(item.Subtitle, "")
		addSubtitle(item.SubtitleAlt, "alt")
		addSubtitle(item.SubtitleCmd, "cmd")
		addSubtitle(item.SubtitleCtrl, "ctrl")
		addSubtitle(item.SubtitleFn, "fn")
		addSubtitle(item.SubtitleShift, "shift")

		data, err := xml.Marshal(xmlItem)
		if err != nil {
			log.Fatalf("ToXML Error: %v\n", err)
		}
		newxml += string(data)
	}

	newxml += "</items>"
	return newxml
}

func FuzzyMatches(val string, test string) bool {
	if test == "" {
		return true
	}

	lval := strings.ToLower(val)
	ltest := strings.ToLower(test)

	words := strings.Split(ltest, " ")
	lastIndex := 0
	for _, word := range words {
		wi := strings.Index(lval, word)
		if wi < lastIndex {
			return false
		}
		lastIndex = wi
	}

	return true
}

type Workflow struct {
	bundleId string
	cacheDir string
	dataDir  string
}

func OpenWorkflow(workflowDir string, createDirs bool) (*Workflow, error) {
	pl, err := plist.UnmarshalFile("info.plist")
	if err != nil {
		log.Println("alfred: Error opening plist:", err)
	}

	plData := pl.Root.(plist.Dict)
	bundleId := plData["bundleid"].(string)
	user, err := user.Current()
	if err != nil {
		return nil, err
	}

	cacheDir := path.Join(user.HomeDir, "Library", "Caches", "com.runningwithcrayons.Alfred-2", "Workflow Data", bundleId)
	dataDir := path.Join(user.HomeDir, "Library", "Application Support", "Alfred 2", "Workflow Data", bundleId)

	if createDirs {
		err := os.MkdirAll(cacheDir, 0755)
		if err != nil {
			return nil, err
		}

		err = os.MkdirAll(dataDir, 0755)
		if err != nil {
			return nil, err
		}
	}

	w := Workflow{
		bundleId: bundleId,
		cacheDir: cacheDir,
		dataDir:  dataDir,
	}

	return &w, nil
}

func GetWorkflow() (*Workflow, error) {
	return OpenWorkflow(".", true)
}

func (w *Workflow) CacheDir() string {
	return w.cacheDir
}

func (w *Workflow) DataDir() string {
	return w.dataDir
}

func (w *Workflow) BundleId() string {
	return w.bundleId
}

func GetConfirmation(title string, prompt string, defaultYes bool) (bool, error) {
	script :=
		`on run argv
		  tell application "Alfred 2"
			  activate
			  set alfredPath to (path to application "Alfred 2")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)

			  try
				display dialog "%s" with title "%s" buttons {"Yes", "No"} default button "%s" with icon alfredIcon
				set answer to (button returned of result)
			  on error number -128
				set answer to "No"
			  end
		  end tell
		end run`
	var def string
	if defaultYes {
		def = "Yes"
	} else {
		def = "No"
	}
	script = fmt.Sprintf(script, prompt, title, def)
	answer, err := RunScript(script)
	if err != nil {
		return false, err
	}

	if strings.TrimSpace(answer) == "Yes" {
		return true, nil
	} else {
		return false, nil
	}
}

func GetInput(title, prompt, defaultVal string, hideAnswer bool) (button, value string, err error) {
	script :=
		`on run argv
		  tell application "Alfred 2"
			  activate
			  set alfredPath to (path to application "Alfred 2")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)

			  try
				display dialog "%s:" with title "%s" default answer "%s" buttons {%s} default button "Ok" with icon alfredIcon %s
				set answer to (button returned of result) & "|" & (text returned of result)
			  on error number -128
				set answer to "Cancel|"
			  end
		  end tell
		end run`

	var hidden string
	if hideAnswer {
		hidden = "with hidden answer"
	}

	script = fmt.Sprintf(script, prompt, title, defaultVal, `"Cancel", "Ok"`, hidden)
	answer, err := RunScript(script)
	if err != nil {
		return button, value, err
	}

	parts := strings.SplitN(answer, "|", 2)

	button = parts[0]
	if len(parts) > 1 {
		value = parts[1]
	}

	return button, value, err
}

func ShowMessage(title string, message string) error {
	script :=
		`on run argv
		  tell application "Alfred 2"
			  activate
			  set alfredPath to (path to application "Alfred 2")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)
			  display dialog "%s" with title "%s" buttons {"Ok"} default button "Ok" with icon alfredIcon
		  end tell
		end run`
	script = fmt.Sprintf(script, message, title)
	_, err := RunScript(script)
	return err
}

func LoadJson(filename string, structure interface{}) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(&structure)
}

func SaveJson(filename string, structure interface{}) error {
	data, _ := json.MarshalIndent(structure, "", "\t")
	log.Println("Saving JSON to", filename)
	return ioutil.WriteFile(filename, data, 0600)
}

func RunScript(script string) (string, error) {
	cmd := exec.Command("osascript", "-")
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

type ByTitle []Item

func (b ByTitle) Len() int {
	return len(b)
}

func (b ByTitle) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b ByTitle) Less(i, j int) bool {
	return b[i].Title < b[j].Title
}

//
// Internal
//

var cacheRoot string
var dataRoot string

func (item *Item) fillSubtitles() {
	if item.SubtitleAlt == "" {
		item.SubtitleAlt = item.Subtitle
	}
	if item.SubtitleCmd == "" {
		item.SubtitleCmd = item.Subtitle
	}
	if item.SubtitleCtrl == "" {
		item.SubtitleCtrl = item.Subtitle
	}
	if item.SubtitleFn == "" {
		item.SubtitleFn = item.Subtitle
	}
	if item.SubtitleShift == "" {
		item.SubtitleShift = item.Subtitle
	}
}

func findFilter(name string, commands []Filter) (f []Filter) {
	for _, c := range commands {
		if name == c.Keyword() {
			f = append(f, c)
		}
	}
	return f
}

func findAction(name string, commands []Action) Action {
	for _, c := range commands {
		if name == c.Keyword() {
			return c
		}
	}
	return nil
}
