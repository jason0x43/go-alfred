package alfred

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
)

// Item is an Alfred list item
type Item struct {
	UID          string
	Title        string
	Subtitle     string
	Autocomplete string
	Arg          *ItemArg
	Icon         string

	mods map[ModKey]*ItemMod
	data *workflowData
}

// ItemArg is an item argument
type ItemArg struct {
	// Keyword specifies the destination keyword for this item
	Keyword string
	// Mode specifies what mode the keyword should operate in
	Mode ModeType
	// Data is the data string that will be passed to the target Command
	Data string
}

// ItemMod is a modifier
type ItemMod struct {
	Arg      *ItemArg
	Subtitle string
}

// JSONItem is the JSON representation of an Alfred item
type jsonItem struct {
	UID          string              `json:"uid,omitempty"`
	Title        string              `json:"title"`
	Subtitle     string              `json:"subtitle,omitempty"`
	Arg          string              `json:"arg,omitempty"`
	Icon         *jsonIcon           `json:"icon,omitempty"`
	Valid        bool                `json:"valid"`
	Autocomplete string              `json:"autocomplete,omitempty"`
	Type         jsonType            `json:"type,omitempty"`
	Mods         map[ModKey]*jsonMod `json:"mods,omitempty"`
	Text         *jsonText           `json:"text,omitempty"`
	QuickLookURL string              `json:"quicklookurl,omitempty"`
}

// jsonType is the type of a JSON item
type jsonType string

const (
	// TypeDefault is a default item
	TypeDefault jsonType = "default"
	// TypeFile is a file item
	TypeFile jsonType = "file"
	// TypeFileSkipCheck is a file item
	TypeFileSkipCheck jsonType = "file:skipcheck"
)

// jsonIcon represents an icon
type jsonIcon struct {
	Type string `json:"type,omitempty"`
	Path string `json:"path"`
}

// jsonMod represents an item subtitle
type jsonMod struct {
	Arg      string `json:"arg,omitempty"`
	Valid    bool   `json:"valid"`
	Subtitle string `json:"subtitle,omitempty"`
}

// jsonText represents an item's optional texts
type jsonText struct {
	Copy      string `json:"copy,omitempty"`
	LargeType string `json:"largetype,omitempty"`
}

// AddMod returns an ItemMod meant to return the user to a previous state
func (i *Item) AddMod(key ModKey, subtitle string, arg *ItemArg) {
	if i.mods == nil {
		i.mods = map[ModKey]*ItemMod{}
	}
	i.mods[key] = &ItemMod{
		Subtitle: subtitle,
		Arg:      arg,
	}
}

// MakeChoice modifies an Item to represent a selectable choice.
func (i *Item) MakeChoice(selected bool) {
	if selected {
		i.Title = "\u2611 " + i.Title
	} else {
		i.Title = "\u2610 " + i.Title
	}
}

// ItemList is a list of items
type ItemList []*Item

// MarshalJSON marshals a list of Items
func (i ItemList) MarshalJSON() ([]byte, error) {
	var jsonItems struct {
		Items []*Item `json:"items"`
	}

	for _, item := range i {
		jsonItems.Items = append(jsonItems.Items, item)
	}

	return json.Marshal(jsonItems)
}

// MarshalJSON marshals an Item into JSON
func (i *Item) MarshalJSON() ([]byte, error) {
	ji := jsonItem{
		UID:          i.UID,
		Title:        i.Title,
		Valid:        i.Arg != nil,
		Autocomplete: i.Autocomplete,
	}

	if i.data == nil {
		i.data = &workflowData{}
	}

	data := i.data

	if i.Arg != nil {
		if i.Arg.Keyword != "" {
			data.Keyword = i.Arg.Keyword
		}

		data.Mode = i.Arg.Mode
		if data.Mode == "" {
			data.Mode = ModeTell
		}

		data.Data = i.Arg.Data
	}

	// Clear the mod flag in case it was set when we got here
	data.Mod = ""

	ji.Arg = Stringify(data)

	if i.Icon != "" {
		ji.Icon = &jsonIcon{
			Path: i.Icon,
		}
	}

	if i.Subtitle != "" {
		ji.Subtitle = i.Subtitle
	}

	if len(i.mods) > 0 {
		ji.Mods = map[ModKey]*jsonMod{}

		for key, mod := range i.mods {
			if mod.Arg != nil {
				data.Mode = mod.Arg.Mode
				data.Data = mod.Arg.Data
			}

			if data.Mode == "" {
				data.Mode = ModeTell
			}

			data.Mod = key

			ji.Mods[key] = &jsonMod{
				Arg:      Stringify(data),
				Valid:    mod.Arg != nil,
				Subtitle: mod.Subtitle,
			}
		}

		data.Mod = ""
	}

	return json.Marshal(ji)
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

	start := strings.IndexRune(lval, rune(ltest[0]))
	if start == -1 {
		return -1.0
	}
	start++

	startScore := 1 - (float64(start) / float64(len(lval)))

	score := 0.20 * startScore

	totalSep := 0
	i := 0

	for _, c := range ltest[1:] {
		if i = strings.IndexRune(lval[start:], c); i == -1 {
			return -1
		}
		totalSep += i
		start += i + 1
	}

	sepScore := math.Max(1-(float64(totalSep)/float64(len(test))), 0)

	score += 0.5 * sepScore

	matchScore := float64(len(test)) / float64(len(val))

	score += 0.2 * matchScore

	return score
}

// InsertItem inserts an item at a specific index in an array of Items.
func InsertItem(items []*Item, item *Item, index int) []*Item {
	items = append(items, item)
	copy(items[index+1:], items[index:])
	items[index] = item
	return items
}

// FuzzySortItems sorts an array of items based how well they match a
// given test string.
func FuzzySortItems(items []*Item, test string) (sorted []*Item) {
	dlog.Printf("fuzzy sorting with: %s", test)
	var sortItems []sortItem
	for i := range items {
		sortItems = append(sortItems, sortItem{
			item: items[i],
			test: test,
		})
	}

	sort.Stable(byFuzzyScore(sortItems))

	for _, si := range sortItems {
		sorted = append(sorted, si.item)
	}

	return
}

// sorting -------------------------------------------------------------------

// ByTitle is an array of Items which will be sorted by title.
type ByTitle []*Item

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

type sortItem struct {
	item   *Item
	score  float64
	scored bool
	test   string
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
		b[i].score = FuzzyScore(b[i].item.Title, b[i].test)
		b[i].scored = true
	}
	if !b[j].scored {
		b[j].score = FuzzyScore(b[j].item.Title, b[j].test)
		b[j].scored = true
	}
	return b[i].score > b[j].score
}
