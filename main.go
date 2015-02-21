// Package alfred provides an API and various utility methods for creating
// Alfred workflows.
package alfred

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"flag"
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
	Line       = "––––––––––––––––––––––––––––––––––––––––––––––"
	Invalid    = "no"
	Valid      = "yes"
	Separator  = " \u00BB" // »
	Terminator = "\u200C"  // zero width joiner
)

type Command interface {
	Keyword() string
	IsEnabled() bool
}

type Filter interface {
	Command
	MenuItem() Item
	Items(prefix, query string) ([]Item, error)
}

type Action interface {
	Command
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

type stringSet map[string]bool

func (s stringSet) Set(value string) error {
	s[value] = true
	return nil
}

func (s *stringSet) String() string {
	return fmt.Sprint(*s)
}

func (w *Workflow) Run(commands []Command) {
	var op string
	var keyword string
	var hideKeyword bool
	var skip = stringSet{}
	var query string
	var err error
	var prefix string

	log.Printf("args: %#v\n", os.Args)

	flag.BoolVar(&hideKeyword, "hide", false, "don't include the keword filter prefixes")
	flag.Var(&skip, "skip", "list of keywords to skip")
	flag.Parse()
	args := flag.Args()

	if len(args) > 0 {
		op = args[0]
	}

	if len(args) > 1 {
		query = args[1]
	}

	if query != "" {
		// take the first word of the query as the keyword
		parts := strings.SplitN(query, " ", 2)
		keyword = strings.TrimSpace(parts[0])
		if len(parts) > 1 {
			query = strings.TrimLeft(parts[1], " ")
		} else {
			query = ""
		}
	}

	if !hideKeyword {
		prefix = keyword + " "
	}

	log.Printf("op: '%s'", op)
	log.Printf("keyword: '%s'", keyword)
	log.Printf("query: '%s'", query)
	log.Printf("prefix: '%s'", prefix)
	log.Printf("skip: '%v'", skip)

	var active []Command
	for _, c := range commands {
		if c.IsEnabled() {
			active = append(active, c)
		}
	}

	switch op {
	case "tell":
		var err error
		var items []Item
		var filters []Filter

		// if we have any filters with an empty keyword, always try them
		emptyFilters := findFilter("", active, stringSet{})
		if len(emptyFilters) > 0 {
			var cmdItems []Item
			for _, f := range emptyFilters {
				cmdItems, err = f.Items("", query)
				if err == nil {
					for _, i := range cmdItems {
						items = append(items, i)
					}
				}
			}
		}

		// only check for filters if we have a keyword; the emptyFilters bit above
		// this has already taken care of an empty keyword
		if keyword != "" {
			filters = findFilter(keyword, active, skip)
		}

		if len(filters) > 0 {
			var cmdItems []Item
			for _, f := range filters {
				cmdItems, err = f.Items(prefix, query)
				if err == nil {
					for _, i := range cmdItems {
						items = append(items, i)
					}
				}
			}
		} else {
			for _, f := range active {
				filter, ok := f.(Filter)
				if ok && f.Keyword() != "" && FuzzyMatches(f.Keyword(), keyword) {
					if _, shouldSkip := skip[f.Keyword()]; !shouldSkip {
						item := filter.MenuItem()
						items = append(items, item)
					}
				}
			}
		}

		if err != nil {
			log.Printf("Error: %s", err)
			items = append(items, Item{Title: fmt.Sprintf("Error: %s", err)})
		} else if len(items) == 0 {
			items = append(items, Item{Title: fmt.Sprintf("Invalid input: %s", query)})
		}

		SendToAlfred(items)

	case "do":
		var output string
		action := findAction(keyword, active, skip)

		if action == nil {
			err = fmt.Errorf("Unknown command '%s'", keyword)
		} else {
			output, err = action.Do(query)
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

// TrimAllLeft returns a copy of an array of strings in which space characters
// are trimmed from the left side of each element in the array.
func TrimAllLeft(parts []string) []string {
	var n []string
	for _, p := range parts {
		n = append(n, strings.TrimLeft(p, " "))
	}
	return n
}

	}
	return parts
}

// Insert an item at a specific index in an array of Items
func InsertItem(items []Item, item Item, index int) []Item {
	items = append(items, item)
	copy(items[index+1:], items[index:])
	items[index] = item
	return items
}

// SendToAlfred sends an array of items to Alfred. Currently this equates to
// outputting an Alfred XML message on stdout.
func SendToAlfred(items []Item) {
	fmt.Println(ToAlfredXML(items))
}

// ToAlfredXML generates an Alfred XML message from an array of items.
func ToAlfredXML(items []Item) string {
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

// Workflow represents an Alfred workflow.
type Workflow struct {
	name     string
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
	name := plData["name"].(string)

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
		name:     name,
		bundleId: bundleId,
		cacheDir: cacheDir,
		dataDir:  dataDir,
	}

	return &w, nil
}

func GetWorkflow() (*Workflow, error) {
	return OpenWorkflow(".", true)
}

// CacheDir returns the cache directory for a workflow.
func (w *Workflow) CacheDir() string {
	return w.cacheDir
}

// DataDir returns the data directory for a workflow.
func (w *Workflow) DataDir() string {
	return w.dataDir
}

// BundleId returns a workflow's bundle ID.
func (w *Workflow) BundleId() string {
	return w.bundleId
}

func (w *Workflow) GetConfirmation(prompt string, defaultYes bool) (bool, error) {
// GetConfirmation opens a confirmation dialog to ask the user to confirm something.
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
	script = fmt.Sprintf(script, prompt, w.name, def)
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

// GetInput opens an input dialog to ask the user for some information.
func (w *Workflow) GetInput(prompt, defaultVal string, hideAnswer bool) (button, value string, err error) {
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

	script = fmt.Sprintf(script, prompt, w.name, defaultVal, `"Cancel", "Ok"`, hidden)
	answer, err := RunScript(script)
	if err != nil {
		return button, value, err
	}

	answer = strings.TrimRight(answer, "\n")
	parts := strings.SplitN(answer, "|", 2)

	button = parts[0]
	if len(parts) > 1 {
		value = parts[1]
	}

	return button, value, err
}

func (w *Workflow) ShowMessage(message string) error {
// ShowMessage opens a message dialog to show the user a message.
	script :=
		`on run argv
		  tell application "Alfred 2"
			  activate
			  set alfredPath to (path to application "Alfred 2")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)
			  display dialog "%s" with title "%s" buttons {"Ok"} default button "Ok" with icon alfredIcon
		  end tell
		end run`
	script = fmt.Sprintf(script, message, w.name)
	_, err := RunScript(script)
	return err
}

// LoadJson reads a JSON file into a provided strucure.
func LoadJson(filename string, structure interface{}) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(&structure)
}

// SaveJson serializes a given structure and saves it to a file.
func SaveJson(filename string, structure interface{}) error {
	data, _ := json.MarshalIndent(structure, "", "\t")
	log.Println("Saving JSON to", filename)
	return ioutil.WriteFile(filename, data, 0600)
}

func RunScript(script string) (string, error) {
	cmd := exec.Command("osascript", "-")
	cmd.Stdin = strings.NewReader(script)
	output, err := cmd.CombinedOutput()
// RunScript runs an arbitrary AppleScript.
	if err != nil {
		return "", err
	}
	return string(output), nil
}

// ByTitle is an array of Items which will be sorted by title.
type ByTitle []Item

// Len returns the length of a ByTitle array.
func (b ByTitle) Len() int {
	return len(b)
}

// Swap swaps two elements in a ByTitle array.
func (b ByTitle) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

// Less indicates whether one ByTitle element should come before another.
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

func findFilter(name string, commands []Command, skip stringSet) (f []Filter) {
	for _, c := range commands {
		filter, ok := c.(Filter)
		if ok && name == c.Keyword() {
			log.Printf("checking " + name)
			if _, shouldSkip := skip[name]; !shouldSkip {
				f = append(f, filter)
			}
		}
	}
	return f
}

func findAction(name string, commands []Command, skip stringSet) Action {
	for _, c := range commands {
		action, ok := c.(Action)
		if ok && name == c.Keyword() {
			if _, shouldSkip := skip[name]; !shouldSkip {
				return action
			}
		}
	}
	return nil
}

