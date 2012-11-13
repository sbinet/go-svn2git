package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gonuts/iochan"
)

var (
	g_help            = flag.Bool("help", false, "display help message and exit")
	g_verbose         = flag.Bool("verbose", false, "")
	g_metadata        = flag.Bool("metadata", false, "include metadata in git logs (git-svn-id)")
	g_no_minimize_url = flag.Bool("no-minimize-url", false, "accept URLs as-is without attempting to connect a higher level directory")
	g_root_is_trunk   = flag.Bool("root-is-trunk", false, "use this if the root level of the repo is equivalent to the trunk and there are no tags or branches")
	g_rebase          = flag.Bool("rebase", false, "instead of cloning a new project, rebase an existing one against SVN")
	g_username        = flag.String("username", "", "username for transports that needs it (http(s), svn)")
	g_trunk           = flag.String("trunk", "trunk", "subpath to trunk from repository URL")
	g_branches        = flag.String("branches", "branches", "subpath to branches from repository URL")
	g_tags            = flag.String("tags", "tags", "subpath to tags from repository URL")
	g_exclude         = flag.String("exclude", "", "regular expression to filter paths when fetching")
	g_revision        = flag.String("revision", "", "start importing from SVN revision START_REV; optionally end at END_REV. e.g. -revision START_REV:END_REV")

	g_no_trunk    = flag.Bool("no-trunk", false, "do not import anything from trunk")
	g_no_branches = flag.Bool("no-branches", false, "do not import anything from branches")
	g_no_tags     = flag.Bool("no-tags", false, "do not import anything from tags")
	g_authors     = flag.String("authors", "$HOME/.config/go-svn2git/authors", "path to file containing svn-to-git authors mapping")

	g_url = ""
)

// repo models a git repository
type repo struct {
	local_branches  []string // the list of local branches
	remote_branches []string // the list of remote branches
	tags            []string // the list of svn-tags
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

func print_cmd(cmd *exec.Cmd) {
	if *g_verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
	}
}

func debug_cmd(cmd *exec.Cmd) {
	if *g_verbose {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
}

func git_cmd(cmdargs ...string) []string {
	lines := []string{}
	cmd := exec.Command("git", cmdargs...)
	if *g_verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		// ignore error
		return lines
	}
	r := bufio.NewReader(bytes.NewBuffer(out))
	for line := range iochan.ReaderChan(r, "\n") {
		lines = append(lines, line)
	}
	return lines
}

func is_in_slice(elmt string, slice []string) bool {
	for _, v := range slice {
		if v == elmt {
			return true
		}
	}
	return false
}

func init() {
	g_repo = repo{
		local_branches:  []string{},
		remote_branches: []string{},
		tags:            []string{},
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
		fmt.Printf(" exclude:  %q\n", *g_exclude)
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
		fmt.Printf(":: --> building list of [local branches]...\n")
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	lines := bufio.NewReader(bytes.NewBuffer(out))
	for line := range iochan.ReaderChan(lines, "\n") {
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
		fmt.Printf(":: --> building list of [remote branches]...\n")
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
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
	if *g_verbose {
		fmt.Printf(":: --> building list of [svn-tags]...\n")
	}
	for _, branch := range g_repo.remote_branches {
		if strings.HasPrefix(branch, "svn/tags/") {
			tag := branch //branch[len("svn/tags/"):]
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
		patterns := []string{}
		if *g_root_is_trunk {
			if *g_trunk != "" {
				patterns = append(patterns, *g_trunk+"[/]")
			}
			if *g_tags != "" {
				patterns = append(patterns, *g_tags+"[/][^/]+[/]")
			}
			if *g_branches != "" {
				patterns = append(patterns, *g_branches+"[/][^/]+[/]")
			}
		}
		regex := fmt.Sprintf("^(?:%s)(?:%s)",
			strings.Join(patterns, "|"),
			*g_exclude)
		cmdargs = append(cmdargs,
			fmt.Sprintf("--ignore-paths=\"%s\"", regex),
		)
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
	usr := make(map[string]string)

	// we only change git config values if g_repo.tags are available.
	// so it stands to reason we should revert them only in that case.
	defer func() {
		if len(g_repo.tags) == 0 {
			return
		}
		for name, v := range usr {
			vv := strings.Trim(v, " ")
			if vv != "" {
				cmd := exec.Command("git", "config", "--local", name,
					strconv.Quote(vv))
				_ = cmd.Run()
			} else {
				cmd := exec.Command("git", "config", "--local", "--unset", name)
				_ = cmd.Run()
			}
			//fmt.Printf("%s: %q %q\n", name, v, vv)
		}
	}()

	git_cfg := func(k string) (string, error) {
		cmd := exec.Command("git", "config", "--local", "--get", k)
		out, err := cmd.CombinedOutput()
		if err != nil {
			// ignore error!
			return "", nil
		}
		r := bufio.NewReader(bytes.NewBuffer(out))
		lines := []string{}
		for line := range iochan.ReaderChan(r, "\n") {
			lines = append(lines, line)
		}
		if len(lines) != 1 {
			return "", fmt.Errorf("too many lines (%d)", len(lines))
		}
		return lines[0], err
	}
	usr["user.name"], _ = git_cfg("user.name")
	usr["user.email"], _ = git_cfg("user.name")

	for itag, tag := range g_repo.tags {
		tag = strings.Trim(tag, " ")
		id := tag[len("svn/tags/"):]
		if *g_verbose {
			hdr := ""
			if itag > 0 {
				hdr = "\n"
			}
			fmt.Printf("%s:: processing svn tag [%s]...\n", hdr, tag)
		}
		subject := git_cmd("log", "-1", "--pretty=format:%s", tag)[0]
		date := git_cmd("log", "-1", "--pretty=format:%ci", tag)[0]
		author := git_cmd("log", "-1", "--pretty=format:%an", tag)[0]
		email := git_cmd("log", "-1", "--pretty=format:%ae", tag)[0]

		cmd := exec.Command("git", "config", "--local", "user.name",
			"\""+author+"\"")
		print_cmd(cmd)
		_ = cmd.Run()

		cmd = exec.Command("git", "config", "--local", "user.email",
			"\""+email+"\"")
		print_cmd(cmd)
		_ = cmd.Run()

		cmd = exec.Command("git", "tag", "-a", "-m",
			fmt.Sprintf("\"%s\"", subject),
			id,
			tag)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_COMMITTER_DATE=%s", date))
		print_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			fmt.Printf("**error** %v\n", err)
			return err
		}

		cmd = exec.Command("git", "branch", "-d", "-r", tag)
		print_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			fmt.Printf("**error** %v\n", err)
			return err
		}

		//fmt.Printf("tag: %q - subject: %q\n", tag, subject)
	}
	return err
}

func fix_branches() error {
	var err error = nil
	svn_branches := []string{}
	for _, v := range g_repo.remote_branches {
		if is_in_slice(v, g_repo.tags) {
			if *g_verbose {
				fmt.Printf("-- discard [%s]...\n", v)
			}
			continue
		}
		if strings.HasPrefix(v, "svn/") {
			svn_branches = append(svn_branches, v)
		}
	}
	if *g_verbose {
		fmt.Printf("svn-branches: %v\n", svn_branches)
	}

	if *g_rebase {
		cmd := exec.Command("git", "svn", "fetch")
		print_cmd(cmd)
		if *g_verbose {
			cmd.Stdin = os.Stdin
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
		}
		err = cmd.Run()
		if err != nil {
			return err
		}
	}

	for _, branch := range svn_branches {
		branch = branch[len("svn/"):]
		if *g_rebase && (is_in_slice(branch, g_repo.local_branches) || branch == "trunk") {
			lbranch := branch
			if branch == "trunk" {
				lbranch = "master"
			}
			cmd := exec.Command("git", "checkout", "-f", lbranch)
			print_cmd(cmd)
			debug_cmd(cmd)
			err = cmd.Run()
			if err != nil {
				return err
			}

			cmd = exec.Command("git", "rebase",
				fmt.Sprintf("remotes/svn/%s", branch),
			)
			print_cmd(cmd)
			debug_cmd(cmd)
			err = cmd.Run()
			if err != nil {
				return err
			}
			continue
		}

		if branch == "trunk" || is_in_slice(branch, g_repo.local_branches) {
			continue
		}

		cmd := exec.Command("git", "branch", "--track", branch,
			fmt.Sprintf("remotes/svn/%s", branch))
		print_cmd(cmd)
		debug_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			return err
		}

		cmd = exec.Command("git", "checkout", branch)
		print_cmd(cmd)
		debug_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	return err
}

func fix_trunk() error {
	var err error = nil
	trunk := ""
	for _, v := range g_repo.remote_branches {
		if strings.Trim(v, " ") == "trunk" {
			trunk = "trunk"
			break
		}
	}
	var cmds []string
	if trunk != "" && !*g_rebase {
		cmds = []string{
			"git checkout svn/trunk",
			"git branch -D master",
			"git checkout -f -b master",
		}
	} else {
		cmds = []string{"git checkout -f master"}
	}
	for _, cmdstr := range cmds {
		cmdargs := strings.Split(cmdstr, " ")
		cmd := exec.Command(cmdargs[0], cmdargs[1:]...)
		print_cmd(cmd)
		debug_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	return err
}

func optimize_repos() error {
	var err error = nil
	cmd := exec.Command("git", "gc")
	print_cmd(cmd)
	debug_cmd(cmd)
	err = cmd.Run()
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
