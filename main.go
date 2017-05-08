// Package alfred provides an API and various utility methods for creating
// Alfred workflows.
package alfred

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"regexp"
	"strings"
)

var dlog = log.New(os.Stderr, "[alfred] ", log.LstdFlags)
var appName string

//
// Public API
//

const (
	// Line is an underline
	Line = "–––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––––"

	// MinAlfredVersion is the minimum supported version of Alfred
	MinAlfredVersion = "3.1"
)

// CleanSplitN trims leading and trailing whitespace from a string, splits it
// into at most N parts, and trims leading and trailing whitespace from each
// part
func CleanSplitN(s, sep string, n int) []string {
	s = strings.Trim(s, " ")
	parts := strings.SplitN(s, sep, n)
	for i, part := range parts {
		parts[i] = strings.Trim(part, " ")
	}
	return parts
}

// IsDebugging indicates whether an Alfred debug panel is open
func IsDebugging() bool {
	return os.Getenv("alfred_debug") == "1"
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

// RunScript runs an arbitrary AppleScript.
func RunScript(script string) (string, error) {
	dlog.Printf("Running script %s", script)
	raw, err := exec.Command("osascript", "-s", "s", "-e", script).CombinedOutput()
	if err != nil {
		dlog.Printf("Error running script: %v", err)
	}
	return strings.TrimRight(string(raw), "\n"), err
}

// SaveJSON serializes a given structure and saves it to a file.
func SaveJSON(filename string, structure interface{}) error {
	data, _ := json.MarshalIndent(structure, "", "\t")
	dlog.Printf("Saving JSON to %s", filename)
	return ioutil.WriteFile(filename, data, 0600)
}

// SplitCmd splits the initial word (a keyword) apart from the rest of an
// argument, returning the keyword (head) and the rest (tail). Whitespace is
// trimmed from both parts.
func SplitCmd(s string) (head, tail string) {
	parts := CleanSplitN(s, " ", 2)
	head = parts[0]
	if len(parts) > 1 {
		tail = parts[1]
	}
	return
}

// Stringify serializes a data object into a string suitable for including in an item Arg
func Stringify(thing interface{}) string {
	if thing == nil {
		return ""
	}

	if str, ok := thing.(string); ok {
		return str
	}

	var bytes []byte
	var err error
	if bytes, err = json.Marshal(thing); err != nil {
		dlog.Fatalf("Error stringifying object: %v", err)
	}

	return string(bytes)
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

// support -------------------------------------------------------------------

// Ensure the workflow environment is initialized
func init() {
	if !IsDebugging() {
		// If a debugging panel isn't open, disable logging
		dlog.SetOutput(ioutil.Discard)
		dlog.SetFlags(0)
	}

	version := os.Getenv("alfred_version")
	dlog.Printf("Alfred version: %s", version)

	if version == "" {
		// If alfred_version wasn't present in the environment, initialize it manually

		plFile := path.Join("workflow", "info.plist")
		if !fileExists(plFile) {
			plFile = "info.plist"
		}
		plData := LoadPlist(plFile)
		bundleID := plData["bundleid"].(string)
		name := plData["name"].(string)

		os.Setenv("alfred_workflow_bundleid", bundleID)
		os.Setenv("alfred_workflow_name", name)

		var version string
		files, _ := ioutil.ReadDir("/Applications")
		var appname string
		for _, file := range files {
			fname := file.Name()
			if fname[0] < 'A' {
				continue
			}
			if fname[0] > 'A' {
				break
			}
			if strings.HasPrefix(fname, "Alfred ") && fname > appname {
				appname = fname
			}
		}

		if appname != "" {
			appname = strings.TrimSuffix(appname, ".app")
			parts := strings.Split(appname, " ")
			if len(parts) == 2 {
				version = parts[1]
				os.Setenv("alfred_short_version", version)
			}
		} else {
			dlog.Fatal("Could not find Alfred app")
		}

		if version == "" {
			dlog.Fatal("Could not determine Alfred version")
		}

		var u *user.User
		var err error
		if u, err = user.Current(); err != nil {
			dlog.Fatal("Error getting user:", err)
		}

		cacheDir := path.Join(u.HomeDir, "Library", "Caches", "com.runningwithcrayons.Alfred-"+version, "Workflow Data", bundleID)
		os.Setenv("alfred_workflow_cache", cacheDir)

		dataDir := path.Join(u.HomeDir, "Library", "Application Support", "Alfred "+version, "Workflow Data", bundleID)
		os.Setenv("alfred_workflow_data", dataDir)
	} else {
		if !checkVersion(version) {
			message := fmt.Sprintf("This workflow requires Alfred %s+", MinAlfredVersion)

			if version[0] == '2' {
				fmt.Printf(`<?xml version="1.0"?><items><item><title>%s</title></item></items>`, message)
			} else {
				fmt.Printf(`{"items":[{"title":"%s"}]}`, message)
			}
			dlog.Fatalf(message)
		}

		os.Setenv("alfred_short_version", strings.SplitN(version, ".", 2)[0])
	}

	appName = "Alfred " + os.Getenv("alfred_short_version")
}

// checkVersion returns true if a given version is greater than or equal to the minimum supported alfred version
func checkVersion(version string) bool {
	validParts := strings.Split(MinAlfredVersion, ".")
	parts := strings.Split(version, ".")

	for i := range validParts {
		if i >= len(parts) || validParts[i] > parts[i] {
			return false
		}
		if parts[i] > validParts[i] {
			return true
		}
	}

	return true
}

func parseDialogResponse(response string) (button string, text string) {
	var parser = regexp.MustCompile(`{button returned:"(\w*)"(?:, text returned:"(.*)")?}`)
	parts := parser.FindStringSubmatch(response)
	if parts != nil {
		button = parts[1]
		text = strings.Replace(parts[2], `\"`, `"`, -1)
	}
	dlog.Printf(`Parsed response: button=%s, text=%s`, button, text)
	return
}

func fileExists(dir string) bool {
	stat, err := os.Stat(dir)
	return !os.IsNotExist(err) && !stat.IsDir()
}
