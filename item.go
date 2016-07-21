package alfred

import (
	"encoding/json"
	"sort"
)

// Item is an Alfred list item
type Item struct {
	UID          string
	Title        string
	Subtitle     string
	Autocomplete string
	Arg          *ItemArg
	Icon         string

	mods map[ModKey]ItemMod
	data workflowData

	// Used for sorting
	fuzzyScore float64
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

// AddMod adds a an ItemMod to an Item's mod map, creating the map if necessary
func (i *Item) AddMod(key ModKey, mod ItemMod) {
	if i.mods == nil {
		i.mods = map[ModKey]ItemMod{}
	}
	i.mods[key] = mod
}

// AddCheckBox modifies an Item to represent a selectable choice.
func (i *Item) AddCheckBox(selected bool) {
	if selected {
		i.Title = "\u2611 " + i.Title
	} else {
		i.Title = "\u2610 " + i.Title
	}
}

// Items is a list of items
type Items []Item

// MarshalJSON marshals a list of Items
func (i Items) MarshalJSON() ([]byte, error) {
	var items struct {
		Items []Item `json:"items"`
	}

	for _, item := range i {
		items.Items = append(items.Items, item)
	}

	return json.Marshal(items)
}

// MarshalJSON marshals an Item into JSON
func (i *Item) MarshalJSON() ([]byte, error) {
	ji := jsonItem{
		UID:          i.UID,
		Title:        i.Title,
		Valid:        i.Arg != nil,
		Autocomplete: i.Autocomplete,
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

// InsertItem inserts an item at a specific index in an array of Items.
func InsertItem(items Items, item Item, index int) Items {
	items = append(items, item)
	copy(items[index+1:], items[index:])
	items[index] = item
	return items
}

// ByTitle is an array of Items which will be sorted by title.
type ByTitle Items

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

// support -------------------------------------------------------------------

// jsonItem is the JSON representation of an Alfred item
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
	typeDefault       jsonType = "default"
	typeFile          jsonType = "file"
	typeFileSkipCheck jsonType = "file:skipcheck"
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

type byFuzzyScore Items

func (b byFuzzyScore) Len() int {
	return len(b)
}

func (b byFuzzyScore) Swap(i, j int) {
	b[i], b[j] = b[j], b[i]
}

func (b byFuzzyScore) Less(i, j int) bool {
	return b[i].fuzzyScore > b[j].fuzzyScore
}

// fuzzySort sorts an array of items in-place based how well they match a given
// test string.
func (i Items) fuzzySort(test string) {
	for idx := range i {
		i[idx].fuzzyScore = fuzzyScore(i[idx].Title, test)
	}
	sort.Stable(byFuzzyScore(i))
}
