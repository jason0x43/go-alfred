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
	_log "log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"regexp"
	"sort"
	"strings"
	"text/template"

	"github.com/jason0x43/go-plist"
)

var log = _log.New(os.Stderr, "[alfred] ", _log.LstdFlags)

//
// Public API
//

const (
	// Line is an underline
	Line = "––––––––––––––––––––––––––––––––––––––––––––––"
	// Invalid flags an invalid item
	Invalid = "no"
	// Valid flags a valid (actionable) item
	Valid = "yes"
	// Separator separates query items/fields
	Separator = " \u00BB" // »
	// Terminator invisibly terminates a query
	Terminator = "\u200C" // zero width joiner
)

// Command is a Filter or Action
type Command interface {
	Keyword() string
	IsEnabled() bool
}

// Filter is a script filter
type Filter interface {
	Command
	MenuItem() Item
	Items(prefix, query string) ([]Item, error)
}

// Action is an action a workflow can perform
type Action interface {
	Command
	Do(query string) (string, error)
}

// Item is an item in an Alfred response
type Item struct {
	UID           string // UID
	Arg           string // Arg
	Title         string // Title
	Subtitle      string // Subtitle
	SubtitleAll   string // SubtitleAll
	SubtitleShift string // SubtitleShift
	SubtitleAlt   string // SubtitleAlt
	SubtitleCmd   string // SubtitleCmd
	SubtitleCtrl  string // SubtitleCtrl
	SubtitleFn    string // SubtitleFn
	Icon          string // Icon
	Valid         string // Valid
	Autocomplete  string // Autocomplete
}

// XMLItem is the XML representation of an Alfred item
type XMLItem struct {
	XMLName      xml.Name      `xml:"item"`
	UID          string        `xml:"uid,attr,omitempty"`
	Title        string        `xml:"title"`
	Subtitles    []XMLSubtitle `xml:"subtitle,omitempty"`
	Icon         string        `xml:"icon,omitempty"`
	Arg          string        `xml:"arg,attr,omitempty"`
	Valid        string        `xml:"valid,attr,omitempty"`
	Autocomplete string        `xml:"autocomplete,attr,omitempty"`
}

// XMLSubtitle represents an item subtitle
type XMLSubtitle struct {
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

// Run runs a workflow.
func (w *Workflow) Run(commands []Command) {
	var op string
	var keyword string
	var hideKeyword bool
	var skip = stringSet{}
	var query string
	var err error
	var prefix string

	log.Printf("args: %#v\n", os.Args)

	flag.BoolVar(&hideKeyword, "hide", false, "don't include the keyword filter prefixes")
	flag.Var(&skip, "skip", "list of keywords to skip")
	flag.Parse()
	args := flag.Args()

	if len(args) > 0 {
		op = args[0]
	}

	if len(args) > 1 {
		query = strings.Join(args[1:], " ")
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
			items = SortItemsForKeyword(items, keyword)
		}

		if err != nil {
			log.Printf("Error: %s", err)
			items = append(items, Item{Title: fmt.Sprintf("Error: %s", err)})
		} else if len(items) == 0 {
			items = append(items, Item{Title: fmt.Sprintf("No results")})
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

// NewKeywordItem creates a new Item representing a keyword.
func NewKeywordItem(keyword, prefix, suffix, desc string) Item {
	return Item{
		Title:        keyword,
		Autocomplete: prefix + keyword + suffix,
		Valid:        Invalid,
		SubtitleAll:  desc,
	}
}

// InsertItem inserts an item at a specific index in an array of Items.
func InsertItem(items []Item, item Item, index int) []Item {
	items = append(items, item)
	copy(items[index+1:], items[index:])
	items[index] = item
	return items
}

// MakeChoice modifies an Item to represent a selectable choice.
func MakeChoice(item Item, selected bool) Item {
	if selected {
		// ballot box with check
		item.Title = "\u2611 " + item.Title
	} else {
		// ballot box
		item.Title = "\u2610 " + item.Title
	}
	return item
}

// SortItemsForKeyword sorts an array of items based how well they match a
// given keyword.
func SortItemsForKeyword(items []Item, keyword string) (sorted []Item) {
	log.Printf("sorting by keyword: %s", keyword)
	var sortItems []sortItem
	for i := range items {
		sortItems = append(sortItems, sortItem{
			item:    &items[i],
			keyword: keyword,
		})
	}

	sort.Stable(byFuzzyScore(sortItems))

	for _, si := range sortItems {
		sorted = append(sorted, *si.item)
	}

	return
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
		xmlItem := XMLItem{
			UID:          item.UID,
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
				xmlItem.Subtitles = append(xmlItem.Subtitles, XMLSubtitle{mod, st})
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

// FuzzyMatches returns true if val and test have a fuzzy match score != -1
func FuzzyMatches(val string, test string) bool {
	return FuzzyScore(val, test) >= 0
}

// FuzzyScore gives a score for how well the test script fuzzy matches a
// given value. To match, the test string must be equal to, or its characters
// must be an ordered subset of, the characters in the val string. A score of 0
// is a perfect match. Higher scores are lower quality matches. A score < 0
// indicates no match.
func FuzzyScore(val string, test string) float64 {
	if test == "" {
		return 0
	}

	lval := strings.ToLower(val)
	ltest := strings.ToLower(test)

	// score -- earlier, closer (average character distance), and higher
	// match-to- ratio == better score

	start := strings.IndexRune(lval, rune(ltest[0]))
	if start == -1 {
		return -1.0
	}
	start++

	// 20% of base score is distance through word that first match occured
	score := 0.20 * float64(start)

	totalSep := 0
	i := 0

	for _, c := range ltest[1:] {
		if i = strings.IndexRune(lval[start:], c); i == -1 {
			return -1
		}
		totalSep += i
		start += i + 1
	}

	// 50% of score is average distance between matching characters
	score += 0.5 * (float64(totalSep) / float64(len(test)))

	// 20% of score is percentage of characters not matched
	score += 0.2 * (float64(len(val)-len(test)) / float64(len(val)))

	log.Printf("score for %s vs %s: %v", val, test, score)

	return score
}

// Workflow represents an Alfred workflow.
type Workflow struct {
	name     string
	bundleID string
	cacheDir string
	dataDir  string
}

// GetAlfredVersion returns the highest installed version of Alfred. It uses a very naive algorithm.
func GetAlfredVersion() string {
	files, _ := ioutil.ReadDir("/Applications")
	name := ""
	for _, file := range files {
		fname := file.Name()
		if strings.HasPrefix(fname, "Alfred ") && fname > name {
			name = fname
			break
		}
	}
	if name != "" {
		name = strings.TrimSuffix(name, ".app")
		parts := strings.Split(name, " ")
		if len(parts) == 2 {
			return parts[1]
		}
	}
	return ""
}

// OpenWorkflow returns a Workflow for a given directory. If the createDirs
// option is true, cache and data directories will be created for the workflow.
func OpenWorkflow(workflowDir string, createDirs bool) (w Workflow, err error) {
	pl, err := plist.UnmarshalFile("info.plist")
	if err != nil {
		log.Fatal("alfred: Error opening plist:", err)
	}

	plData := pl.Root.(plist.Dict)
	bundleID := plData["bundleid"].(string)
	name := plData["name"].(string)
	version := GetAlfredVersion()

	var u *user.User
	if u, err = user.Current(); err != nil {
		return
	}

	cacheDir := path.Join(u.HomeDir, "Library", "Caches", "com.runningwithcrayons.Alfred-"+version, "Workflow Data", bundleID)
	dataDir := path.Join(u.HomeDir, "Library", "Application Support", "Alfred "+version, "Workflow Data", bundleID)

	if createDirs {
		if err = os.MkdirAll(cacheDir, 0755); err != nil {
			return
		}
		if err = os.MkdirAll(dataDir, 0755); err != nil {
			return
		}
	}

	w = Workflow{
		name:     name,
		bundleID: bundleID,
		cacheDir: cacheDir,
		dataDir:  dataDir,
	}

	return
}

// CacheDir returns the cache directory for a workflow.
func (w *Workflow) CacheDir() string {
	return w.cacheDir
}

// DataDir returns the data directory for a workflow.
func (w *Workflow) DataDir() string {
	return w.dataDir
}

// BundleID returns a workflow's bundle ID.
func (w *Workflow) BundleID() string {
	return w.bundleID
}

// GetConfirmation opens a confirmation dialog to ask the user to confirm something.
func (w *Workflow) GetConfirmation(prompt string, defaultYes bool) (confirmed bool, err error) {
	version := GetAlfredVersion()
	type ScriptData struct {
		Version string
		Prompt  string
		Title   string
		Default string
	}

	script :=
		`tell application "Alfred {{.Version}}"
			  activate
			  set alfredPath to (path to application "Alfred {{.Version}}")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)
			  display dialog "{{.Prompt}}" with title "{{.Title}}" buttons {"Yes", "No"} default button "{{.Default}}" with icon alfredIcon
		  end tell`

	data := ScriptData{version, prompt, w.name, "No"}
	if defaultYes {
		data.Default = "Yes"
	}

	var tmpl *template.Template
	tmpl, err = template.New("script").Parse(script)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return
	}

	script = buf.String()
	var response string
	response, err = RunScript(script)
	if err != nil {
		return
	}

	button, _ := parseDialogResponse(response)
	return button == "Yes", nil
}

// GetInput opens an input dialog to ask the user for some information.
func (w *Workflow) GetInput(prompt, defaultVal string, hideAnswer bool) (button, value string, err error) {
	version := GetAlfredVersion()
	type ScriptData struct {
		Version string
		Prompt  string
		Title   string
		Default string
		Hidden  string
	}

	script :=
		`tell application "Alfred {{.Version}}"
			  activate
			  set alfredPath to (path to application "Alfred {{.Version}}")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)
			  display dialog "{{.Prompt}}:" with title "{{.Title}}" default answer "{{.Default}}" buttons {"Cancel", "Ok"} default button "Ok" with icon alfredIcon{{.Hidden}}
		  end tell`

	data := ScriptData{version, prompt, w.name, defaultVal, ""}
	if hideAnswer {
		data.Hidden = " with hidden answer"
	}

	var tmpl *template.Template
	tmpl, err = template.New("script").Parse(script)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return
	}

	script = buf.String()
	var response string
	response, err = RunScript(script)
	log.Printf("got response: '%s'", response)
	if err != nil {
		if strings.Contains(response, "User canceled") {
			log.Printf("User canceled")
			return "Cancel", "", nil
		}
		return
	}

	button, value = parseDialogResponse(response)
	return
}

// ShowMessage opens a message dialog to show the user a message.
func (w *Workflow) ShowMessage(message string) (err error) {
	version := GetAlfredVersion()
	type ScriptData struct {
		Version string
		Prompt  string
		Title   string
	}

	script :=
		`tell application "Alfred {{.Version}}"
			  activate
			  set alfredPath to (path to application "Alfred {{.Version}}")
			  set alfredIcon to path to resource "appicon.icns" in bundle (alfredPath as alias)
			  display dialog "{{.Prompt}}" with title "{{.Title}}" buttons {"Ok"} default button "Ok" with icon alfredIcon
		  end tell`

	data := ScriptData{version, message, w.name}

	var tmpl *template.Template
	tmpl, err = template.New("script").Parse(script)
	if err != nil {
		return
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return
	}

	script = buf.String()
	_, err = RunScript(script)
	return
}

// LoadJSON reads a JSON file into a provided strucure.
func LoadJSON(filename string, structure interface{}) error {
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		return err
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	return dec.Decode(&structure)
}

// SaveJSON serializes a given structure and saves it to a file.
func SaveJSON(filename string, structure interface{}) error {
	data, _ := json.MarshalIndent(structure, "", "\t")
	log.Printf("Saving JSON to %s", filename)
	return ioutil.WriteFile(filename, data, 0600)
}

// RunScript runs an arbitrary AppleScript.
func RunScript(script string) (string, error) {
	raw, err := exec.Command("osascript", "-s", "s", "-e", script).CombinedOutput()
	return strings.TrimRight(string(raw), "\n"), err
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

func parseDialogResponse(response string) (button string, text string) {
	var parser = regexp.MustCompile(`{button returned:"(\w*)"(?:, text returned:"(.*)")?}`)
	parts := parser.FindStringSubmatch(response)
	if parts != nil {
		button = parts[1]
		text = strings.Replace(parts[2], `\"`, `"`, -1)
	}
	log.Printf(`Parsed response: button=%s, text=%s`, button, text)
	return
}

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

type sortItem struct {
	item    *Item
	score   float64
	scored  bool
	keyword string
}

type byFuzzyScore []sortItem

func (b byFuzzyScore) Len() int {
	return len(b)
}

func (b byFuzzyScore) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFuzzyScore) Less(i, j int) bool {
	if !b[i].scored {
		b[i].score = FuzzyScore(b[i].item.Title, b[i].keyword)
		b[i].scored = true
	}
	if !b[j].scored {
		b[j].score = FuzzyScore(b[j].item.Title, b[j].keyword)
		b[j].scored = true
	}
	return b[i].score < b[j].score
}
