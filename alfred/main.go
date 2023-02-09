// The alfred command can be used to manage Go-based Alfred workflows.
//
// The command must be run from a workflow directory, a directory containing a
// "workflow" subdirectory. The basename of the workflow directory is the
// workflow's filename. A typical layout would look like:
//
//	my-workflow/
//		README.md
//		LICENSE.txt
//		main.go
//		workflow/
//			info.plist
//			icon.png
//
// Installation:
//
//	go install github.com/jason0x43/go-alfred/alfred
//
// Usage:
//
//	alfred [command] [options]
//
// The available commands are:
//
//	build
//		Build the workflow executable and output it into the "workflow"
//		subdirectory.
//	clean
//		Delete the compiled workflow executable and the workflow distributable
//		package.
//	info
//		Display information about the workflow.
//	link
//		Link the "workflow" subdirectory into Alfred's preferences directory,
//		installing it.
//	pack [outdir]
//		Package the workflow for distribution. This will create a file named
//		<filename>.alfredworkflow, where "filename" is the basename of the
//		workflow directory.
//	release [outdir]
//		Prepare the repo for release.
//	unlink
//		Unlink the "workflow" subdirectory from Alfred's preferences directory,
//		uninstalling it.
package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"

	"github.com/Masterminds/semver"
	"github.com/jason0x43/go-alfred"
)

var workflowName string
var zipName string
var workflowPath string
var workflowsPath string
var buildDir = "workflow"

type command struct {
	Name    string
	Options string
	Help    string
}

var commands = []command{
	{"build", "", "build the workflow executable (-a to rebuild libs)"},
	{"clean", "", "clean built files"},
	{"help", "", "display this help message"},
	{"info", "", "display information about the current workflow"},
	{"link", "", "activate this workflow"},
	{"pack", "[outdir]", "create a distributable package"},
	{"release", "[outdir]", "create a new release"},
	{"unlink", "", "deactivate this workflow"},
}

var dlog = log.New(os.Stderr, "[alfred] ", log.LstdFlags)

func main() {
	if os.Getenv("alfred_debug") != "1" {
		dlog.SetOutput(ioutil.Discard)
		dlog.SetFlags(0)
	}

	prefsDir := getPrefsDirectory()
	dlog.Printf("prefs dir: %s", prefsDir)
	workflowsPath = path.Join(prefsDir, "Alfred.alfredpreferences/workflows")
	dlog.Printf("workflows path: %s", workflowsPath)

	if len(os.Args) == 1 {
		help()
		os.Exit(0)
	}

	if !dirExists(path.Join("workflow")) {
		dlog.Printf("Didn't see workflow/ in cwd, going up...")
		os.Chdir("..")
		if !dirExists("workflow") {
			println("You're not in a workflow.")
			os.Exit(1)
		}
	}

	workflowPath, _ = filepath.Abs(".")
	workflowName = path.Base(workflowPath)

	plistFile := path.Join("workflow", "info.plist")
	versionTag := ""

	if fileExists(plistFile) {
		infoPlist := alfred.LoadPlist(plistFile)
		workflowVersion := infoPlist["version"]
		if workflowVersion != nil {
			versionTag = fmt.Sprintf("-%s", workflowVersion)
		}
	}

	zipName = fmt.Sprintf("%s%s.alfredworkflow", workflowName, versionTag)
	dlog.Printf("zipName: %s", zipName)

	switch os.Args[1] {
	case "build":
		build()
	case "clean":
		clean()
	case "help":
		help()
	case "info":
		info()
	case "link":
		link()
	case "pack":
		pack()
	case "release":
		release()
	case "unlink":
		unlink()
	default:
		println("Unknown command:", os.Args[1])
	}
}

// getAlfredVersion returns the highest installed version of Alfred. It uses a very naive algorithm.
func getAlfredVersion() string {
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

func run(cmd string, args ...string) {
	if output, err := exec.Command(cmd, args...).CombinedOutput(); err != nil {
		println(string(output))
		panic(err)
	}
}

func runIfFile(file, cmd string, args ...string) {
	if _, err := os.Stat(file); err == nil {
		run(cmd, args...)
	}
}

func getPrefsDirectory() string {
	currentUser, _ := user.Current()

	version := getAlfredVersion()
	prefSuffix := ""
	if version != "2" && version != "4" && version != "5" {
		prefSuffix = "-" + version
	}

	prefFile := path.Join(currentUser.HomeDir, "Library", "Preferences",
		"com.runningwithcrayons.Alfred-Preferences"+prefSuffix+".plist")
	preferences := alfred.LoadPlist(prefFile)

	var folder string

	if preferences["syncfolder"] != nil && preferences["syncfolder"] != "" {
		folder = preferences["syncfolder"].(string)
		if strings.HasPrefix(folder, "~/") {
			folder = path.Join(currentUser.HomeDir, folder[2:])
		}
	} else {
		folder = path.Join(currentUser.HomeDir, "Library", "Application Support", "Alfred")
		if !dirExists(folder) {
			folder = path.Join(currentUser.HomeDir, "Library", "Application Support", "Alfred "+version)
		}
	}

	var info os.FileInfo
	var err error
	if info, err = os.Stat(folder); err != nil {
		panic(err)
	}

	if !info.IsDir() {
		panic(fmt.Errorf("%s is not a directory", folder))
	}

	return folder
}

func loadPreferences() (prefs alfred.Plist) {
	currentUser, _ := user.Current()

	version := getAlfredVersion()
	prefSuffix := ""
	if version != "2" {
		prefSuffix = "-" + version
	}

	prefFile := path.Join(currentUser.HomeDir, "Library", "Preferences",
		"com.runningwithcrayons.Alfred-Preferences"+prefSuffix+".plist")
	return alfred.LoadPlist(prefFile)
}

func build() {
	command := flag.NewFlagSet("build", flag.ExitOnError)
	help := command.Bool("h", false, "show this message")
	command.Parse(os.Args[2:])

	if *help {
		dlog.Printf("Showing help")
		command.PrintDefaults()
		os.Exit(0)
	}

	dlog.Printf("Building the workflow...")

	// use go generate, along with custom build tools, to handle any auxiliary
	// build steps
	run("go", "generate")

	cmdAmd64 := exec.Command("go", "build", "-ldflags", "-s -w", "-o", workflowName+"_amd64")
	cmdAmd64.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=amd64")
	if output, err := cmdAmd64.CombinedOutput(); err != nil {
		println(string(output))
		panic(err)
	}
	cmdArm64 := exec.Command("go", "build", "-ldflags", "-s -w", "-o", workflowName+"_arm64")
	cmdArm64.Env = append(os.Environ(), "GOOS=darwin", "GOARCH=arm64")
	if output, err := cmdArm64.CombinedOutput(); err != nil {
		println(string(output))
		panic(err)
	}

	run(
		"lipo",
		"-create",
		"-output",
		"workflow/"+workflowName,
		workflowName+"_amd64",
		workflowName+"_arm64",
	)

	run("rm", workflowName+"_amd64")
	run("rm", workflowName+"_arm64")
}

func clean() {
	dlog.Printf("Cleaning the workflow...")
	binFile := path.Join("workflow", workflowName)
	if _, err := os.Stat(binFile); err == nil {
		run("rm", binFile)
	}
	if _, err := os.Stat(zipName); err == nil {
		run("rm", zipName)
	}
}

func getExistingLink() (string, error) {
	dir, err := os.Open(workflowsPath)
	if err != nil {
		return "", err
	}
	defer dir.Close()

	dirs, err := dir.Readdir(-1)
	if err != nil {
		return "", err
	}

	wd, _ := os.Getwd()
	buildPath := path.Join(wd, buildDir)

	for _, dir := range dirs {
		if dir.Mode()&os.ModeSymlink == os.ModeSymlink {
			fullDir := path.Join(workflowsPath, dir.Name())
			link, err := filepath.EvalSymlinks(fullDir)
			if err == nil && link == buildPath {
				return fullDir, nil
			}
		}
	}

	return "", nil
}

func getExistingInstall() (string, error) {
	dir, err := os.Open(workflowsPath)
	if err != nil {
		return "", err
	}
	defer dir.Close()

	plistFile := path.Join("workflow", "info.plist")
	info := alfred.LoadPlist(plistFile)
	id := info["bundleid"]

	dirs, err := dir.Readdir(-1)
	if err != nil {
		return "", err
	}

	for _, d := range dirs {
		infoFile := path.Join(dir.Name(), d.Name(), "info.plist")
		if !fileExists(infoFile) {
			continue
		}

		infoPlist := alfred.LoadPlist(infoFile)
		workflowID := infoPlist["bundleid"]
		if workflowID == id {
			return d.Name(), nil
		}
	}

	return "", nil
}

func help() {
	println("usage:", os.Args[0], "<command> [options]")
	println()
	println("command may be one of:")
	for _, cmd := range commands {
		fmt.Printf("    %-18s %s\n", cmd.Name+" "+cmd.Options, cmd.Help)
	}
}

func info() {
	dlog.Printf("Getting workflow info...")
	width := -15

	printField := func(name, value string) {
		fmt.Printf("%*s %s\n", width, name+":", value)
	}

	printField("Workflows", workflowsPath)

	if link, _ := getExistingLink(); link != "" {
		printField("This workflow", path.Base(link))
	}

	plistFile := path.Join("workflow", "info.plist")
	info := alfred.LoadPlist(plistFile)
	printField("Version", info["version"].(string))
}

func link() {
	dlog.Printf("Linking workflow...")
	existing, err := getExistingLink()
	if err != nil {
		panic(err)
	}

	if existing != "" {
		println("Existing link", filepath.Base(existing))
		return
	}

	existing, err = getExistingInstall()
	if err != nil {
		panic(err)
	}

	if existing != "" {
		plistFile := path.Join(workflowsPath, existing, "info.plist")
		dlog.Printf("Reading from plist file %s", plistFile)
		info := alfred.LoadPlist(plistFile)
		info["disabled"] = true
		alfred.SavePlist(plistFile, info)
		println("disabled existing install at", existing)
	}

	uuidgen, _ := exec.Command("uuidgen").Output()
	uuid := strings.TrimSpace(string(uuidgen))
	target := path.Join(workflowsPath, "user.workflow."+string(uuid))
	dlog.Printf("Creating new link to target %s", target)
	buildPath := path.Join(workflowPath, buildDir)
	dlog.Printf("Build path is %s", buildPath)
	run("ln", "-s", buildPath, target)
	println("created link", filepath.Base(target))
}

func pack() {
	command := flag.NewFlagSet("build", flag.ExitOnError)
	help := command.Bool("h", false, "show this message")
	outdir := command.String("o", "", "output directory")
	command.Parse(os.Args[2:])

	if *help {
		dlog.Printf("Showing help")
		command.PrintDefaults()
		os.Exit(0)
	}

	dlog.Printf("Packing workflow...")

	if err := createArchive(*outdir); err != nil {
		panic(err)
	}
}

func createArchive(outdir string) error {
	if outdir != "" {
		outdir, _ = filepath.Abs(outdir)
	} else {
		outdir = ".."
	}

	pwd, _ := filepath.Abs(".")

	if err := os.Chdir(buildDir); err != nil {
		return err
	}

	zipfile := path.Join(outdir, zipName)
	dlog.Printf("Creating archive %s", zipfile)
	run("zip", "-r", zipfile, ".")

	if err := os.Chdir(pwd); err != nil {
		return err
	}

	return nil
}

func release() {
	command := flag.NewFlagSet("build", flag.ExitOnError)
	help := command.Bool("h", false, "show this message")
	outdir := command.String("o", "", "output directory")
	userVersion := command.String("v", "", "release version")
	command.Parse(os.Args[2:])

	if *help {
		dlog.Printf("Showing help")
		command.PrintDefaults()
		os.Exit(0)
	}

	dlog.Printf("Releasing workflow...")
	plistFile := path.Join("workflow", "info.plist")
	dlog.Printf("Reading from plist file %s", plistFile)
	info := alfred.LoadPlist(plistFile)
	var version semver.Version
	var releaseVersion string

	if *userVersion != "" {
		version = *semver.MustParse(*userVersion)
		releaseVersion = version.String()
		dlog.Printf("Using user-provided version: %s", releaseVersion)
	} else {
		version = *semver.MustParse(info["version"].(string))
		dlog.Printf("Using version from info.plist: %s", info["version"].(string))
		if version.Prerelease() != "" {
			releaseVer, _ := version.SetPrerelease("")
			releaseVersion = releaseVer.String()
			dlog.Printf("Release version is: %s", releaseVersion)
		} else {
			panic("Workflow version must be a prerelease, or a new version must be specified")
		}
	}

	fmt.Printf("Updating version to %s for release\n", releaseVersion)
	info["version"] = releaseVersion
	alfred.SavePlist(plistFile, info)
	dlog.Printf("Saved plist")
	run("git", "commit", "-a", "-m", fmt.Sprintf("Update version to %s for release", releaseVersion))
	dlog.Printf("Commited changes to repo")
	run("git", "tag", releaseVersion)
	dlog.Printf("Tagged release")
	fmt.Printf("Packaging version %s\n", releaseVersion)
	build()

	if err := createArchive(*outdir); err != nil {
		panic(err)
	}

	nextVer, _ := version.IncMinor().SetPrerelease("pre")
	nextVersion := nextVer.String()
	fmt.Printf("Updating version to %s\n", nextVersion)
	info["version"] = nextVersion
	alfred.SavePlist(plistFile, info)
	run("git", "commit", "-a", "-m", fmt.Sprintf("Update version to %s", nextVersion))

	fmt.Printf("Done!\n")
}

func unlink() {
	dlog.Printf("Unlinkin workflow...")
	existing, err := getExistingLink()
	if err != nil {
		panic(err)
	}

	if existing == "" {
		return
	}

	run("rm", existing)
	println("removed link", filepath.Base(existing))

	if existing, err = getExistingInstall(); err != nil {
		panic(err)
	}

	if existing != "" {
		plistFile := path.Join(workflowsPath, existing, "info.plist")
		info := alfred.LoadPlist(plistFile)
		info["disabled"] = false
		alfred.SavePlist(plistFile, info)
		println("enabled existing install at", existing)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}

func copyFiles(srcDir, dstDir string) error {
	entries, err := ioutil.ReadDir(srcDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		srcPath := path.Join(srcDir, entry.Name())
		dstPath := path.Join(dstDir, entry.Name())

		if entry.IsDir() {
			os.Mkdir(dstPath, 0777)
			if err := copyFiles(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func dirExists(dir string) bool {
	stat, err := os.Stat(dir)
	if err != nil {
		return os.IsExist(err)
	}
	return stat.IsDir()
}

func fileExists(file string) bool {
	stat, err := os.Stat(file)
	if err != nil {
		return os.IsExist(err)
	}
	return !stat.IsDir()
}
