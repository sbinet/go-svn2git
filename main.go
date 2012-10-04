package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

var g_verbose = flag.Bool("verbose", false, "")
var g_metadata = flag.Bool("metadata", false, "include metadata in git logs (git-svn-id)")
var g_no_minimize_url = flag.Bool("no-minimize-url", false, "accept URLs as-is without attempting to connect a higher level directory")
var g_root_is_trunk = flag.Bool("root-is-trunk", false, "use this if the root level of the repo is equivalent to the trunk and there are no tags or branches")
var g_rebase = flag.Bool("rebase", false, "instead of cloning a new project, rebase an existing one against SVN")
var g_username = flag.String("username", "", "username for transports that needs it (http(s), svn)")
var g_trunk = flag.String("trunk", "trunk", "subpath to trunk from repository URL")
var g_branches = flag.String("branches", "branches", "subpath to branches from repository URL")
var g_tags = flag.String("tags", "tags", "subpath to tags from repository URL")
var g_exclude = flag.String("exclude", "", "regular expression to filter paths when fetching")
var g_revision = flag.String("revision", "", "start importing from SVN revision START_REV; optionally end at END_REV. e.g. -revision START_REV:END_REV")

var g_no_trunk = flag.Bool("no-trunk", false, "do not import anything from trunk")
var g_no_branches = flag.Bool("no-branches", false, "do not import anything from branches")
var g_no_tags = flag.Bool("no-tags", false, "do not import anything from tags")
var g_authors = flag.String("authors", "~/.config/go-svn2git/authors", "path to file containing svn-to-git authors mapping")

var g_url = ""

// repo models a git repository
type repo struct {
	local_branches []string // the list of local branches
	remote_branches []string // the list of remote branches
	tags []string // the list of svn-tags
}

var g_repo repo

func path_exists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

func init() {
	g_repo = repo{
		local_branches: []string{},
		remote_branches: []string{},
		tags: []string{},
	}
}

func main() {
	flag.Parse()

	if *g_root_is_trunk {
		*g_trunk = ""
		*g_branches = ""
		*g_tags = ""
	}

	if *g_no_trunk {
		*g_trunk = ""
	}

	if *g_no_branches {
		*g_branches = ""
	}

	if *g_no_tags {
		*g_tags = ""
	}

	*g_authors = os.ExpandEnv(*g_authors)
	if !path_exists(*g_authors) {
		*g_authors = ""
	}

	if *g_verbose {
		fmt.Printf("==go-svn2git...\n")
		fmt.Printf(" verbose:  %v\n", *g_verbose)
		fmt.Printf(" rebase:   %v\n", *g_rebase)
		fmt.Printf(" username: %q\n", *g_username)
		fmt.Printf(" trunk:    %q\n", *g_trunk)
		fmt.Printf(" branches: %q\n", *g_branches)
		fmt.Printf(" tags:     %q\n", *g_tags)
		fmt.Printf(" authors:  %q\n", *g_authors)
		fmt.Printf(" root-is-trunk: %v\n", *g_root_is_trunk)

	}

	if *g_rebase {
		if flag.NArg() > 0 {
			fmt.Printf("** too many arguments\n")
			flag.Usage()
			err := verify_working_tree_is_clean()
			if err != nil {
				os.Exit(1)
			}
		}
	} else {
		ok := true
		switch flag.NArg() {
		case 0:
			fmt.Printf("** missing SVN_URL parameter\n")
			ok = false
		case 1:
			/*noop*/
		default:
			fmt.Printf("** too many arguments\n")
			ok = false
		}
		if !ok {
			flag.Usage()
			os.Exit(1)
		}
		g_url = flag.Arg(0)
	}

	err := run()
	if err != nil {
		fmt.Printf("**error** %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	var err error = nil
	if *g_rebase {
		err = get_branches()
	} else {
		err = do_clone()
	}
	if err != nil {
		return err
	}

	err = fix_tags()
	if err != nil {
		return err
	}

	err = fix_branches()
	if err != nil {
		return err
	}

	err = fix_trunk()
	if err != nil {
		return err
	}

	err = optimize_repos()
	if err != nil {
		return err
	}

	return err
}

func get_branches() error {
	var err error = nil
	// get the list of local and remote branches
	// ignore console color codes
	cmd := exec.Command("git", "branch", "-l", "--no-color")
	if *g_verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
		// cmd.Stdin = os.Stdin
		// cmd.Stdout = os.Stdout
		// cmd.Stderr = os.Stderr
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	lines := bufio.NewReader(bytes.NewBuffer(out))
	for {
		line, err1 := lines.ReadString('\n')
		if err1 != nil {
			if err1 != io.EOF {
				err = err1
			}
			break
		}
		if *g_verbose {
			fmt.Printf("   %q\n", line)
		}
		if strings.HasPrefix(line, "*") {
			line = strings.Replace(line, "*", "", 1)
		}
		line = strings.Trim(line, " \r\n")
		if *g_verbose {
			fmt.Printf("-> %q\n", line)
		}
		g_repo.local_branches = append(g_repo.local_branches, line)
	}

	// remote branches...
	cmd = exec.Command("git", "branch", "-r", "--no-color")
	if *g_verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
		// cmd.Stdin = os.Stdin
		// cmd.Stdout = os.Stdout
		// cmd.Stderr = os.Stderr
	}
	out, err = cmd.CombinedOutput()
	if err != nil {
		return err
	}
	lines = bufio.NewReader(bytes.NewBuffer(out))
	for {
		line, err1 := lines.ReadString('\n')
		if err1 != nil {
			if err1 != io.EOF {
				err = err1
			}
			break
		}
		if *g_verbose {
			fmt.Printf("   %q\n", line)
		}
		if strings.HasPrefix(line, "*") {
			line = strings.Replace(line, "*", "", 1)
		}
		line = strings.Trim(line, " \r\n")
		if *g_verbose {
			fmt.Printf("-> %q\n", line)
		}
		g_repo.remote_branches = append(g_repo.remote_branches, line)
	}

	// tags are remote branches that start with "svn/tags/"
	for _, branch := range g_repo.remote_branches {
		if strings.HasPrefix(branch, "svn/tags/") {
			tag := branch[len("svn/tags/"):]
			if *g_verbose {
				fmt.Printf(":: adding tag %q...\n", tag)
			}
			g_repo.tags = append(g_repo.tags, tag)

		}
	}
	return err
}

func do_clone() error {
	var err error = nil

	cmdargs := []string{
		"svn", "init", "--prefix=svn/",
	}
	var cmd *exec.Cmd = nil
	if *g_root_is_trunk {
		// non-standard repository layout.
		// The repository root is effectively 'trunk.'
		if *g_username != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--username=%s", *g_username))
		}
		if *g_metadata {
			cmdargs = append(cmdargs, "--no-metadata")
		}
		if *g_no_minimize_url {
			cmdargs = append(cmdargs, "--no-minimize-url")
		}
		cmdargs = append(cmdargs, fmt.Sprintf("--trunk=%s", g_url))
	} else {
		// add each component to the command that was passed as an argument
		if *g_username != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--username=%s", *g_username))
		}
		if *g_metadata {
			cmdargs = append(cmdargs, "--no-metadata")
		}
		if *g_no_minimize_url {
			cmdargs = append(cmdargs, "--no-minimize-url")
		}
		if *g_trunk != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--trunk=%s", *g_trunk))
		}
		if *g_tags != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--tags=%s", *g_tags))
		}
		if *g_branches != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--branches=%s", *g_branches))
		}
		cmdargs = append(cmdargs, g_url)
	}
	cmd = exec.Command("git", cmdargs...)
	if *g_verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err = cmd.Run()
	if err != nil {
		return err
	}

	if *g_authors != "" {
		cmd := exec.Command("git", "config", "--local", "svn.authorsfile",
			*g_authors)

		if *g_verbose {
			fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		err = cmd.Run()
		if err != nil {
			return err
		}

	} else {
		if *g_verbose {
			fmt.Printf("no authors file...\n")
		}
	}

	cmdargs = []string{"svn", "fetch"}
	if *g_revision != "" {
		rev := strings.Split(*g_revision, ":")
		switch len(rev) {
		case 0:
			panic("the impossible happened!")
		case 1:
			if rev[0] == "" {
				return fmt.Errorf("invalid empty argument to '-revision' flag")
			}
			rev = []string{rev[0], "HEAD"}
		case 2:
			if rev[0] == "" {
				rev[0] = "0"
			}
			if rev[1] == "" {
				rev[1] = "HEAD"
			}
		default:
			return fmt.Errorf("invalid argument to '-revision' flag (%q)", *g_revision)
		}
		cmdargs = append(cmdargs,
			"-r",
			fmt.Sprintf("%s:%s", rev[0], rev[1]),
		)
	}
	if *g_exclude != "" {
		fmt.Printf("**warning** -exclude 'REGEX' is NOT implemented\n")
	}

	cmd = exec.Command("git", cmdargs...)
	if *g_verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err = cmd.Run()
	if err != nil {
		return err
	}
	
	err = get_branches()
	if err != nil {
		return err
	}

	return err
}

func fix_tags() error {
	var err error = nil
	return err
}

func fix_branches() error {
	var err error = nil
	return err
}

func fix_trunk() error {
	var err error = nil
	return err
}

func optimize_repos() error {
	var err error = nil
	return err
}

func verify_working_tree_is_clean() error {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=no")
	out, err := cmd.CombinedOutput()
	if len(out) != 0 {
		fmt.Printf("** you have pending changes. The working tree must be clean in order to continue.\n")
	}
	return err
}

// EOF
