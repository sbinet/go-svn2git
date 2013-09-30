package svn

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gonuts/iochan"
)

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

func is_in_slice(elmt string, slice []string) bool {
	for _, v := range slice {
		if v == elmt {
			return true
		}
	}
	return false
}

// Repo models a git repository
type Repo struct {
	local_branches  []string // the list of local branches
	remote_branches []string // the list of remote branches
	tags            []string // the list of svn-tags
}

type Context struct {
	Repo Repo // Git repository we are working on

	Url string // SVN URL to work from

	Verbose       bool
	Metadata      bool   // include metadata in git logs (git-svn-id)
	NoMinimizeUrl bool   // accept URLs as-is without attempting to connect a higher level directory
	RootIsTrunk   bool   // use this if the root level of the repo is equivalent to the trunk and there are no tags or branches
	Rebase        bool   // instead of cloning a new project, rebase an existing one against SVN
	UserName      string // username for transports that needs it (http(s), svn)
	Trunk         string // subpath to trunk from repository URL
	Branches      string // subpath to branches from repository URL
	Tags          string // subpath to tags from repository URL
	Exclude       string // regular expression to filter paths when fetching
	Revision      string // start importing from SVN revision START_REV; optionally end at END_REV. e.g. START_REV:END_REV

	NoTrunk    bool   // do not import anything from trunk
	NoBranches bool   // do not import anything from branches
	NoTags     bool   // do not import anything from tags
	Authors    string // path to file containing svn-to-git authors mapping
}

func NewContext(svnurl string) *Context {
	ctx := &Context{
		Repo: Repo{
			local_branches:  []string{},
			remote_branches: []string{},
			tags:            []string{},
		},
		Url:           svnurl,
		Verbose:       true,
		Metadata:      false,
		NoMinimizeUrl: false,
		RootIsTrunk:   false,
		Rebase:        false,
		UserName:      "",
		Trunk:         "trunk",
		Branches:      "branches",
		Tags:          "tags",
		Exclude:       "",
		Revision:      "",
		NoTrunk:       false,
		NoBranches:    false,
		NoTags:        false,
		Authors:       os.ExpandEnv("$HOME/.config/go-svn2git/authors"),
	}
	if !path_exists(ctx.Authors) {
		ctx.Authors = ""
	}
	return ctx
}

func NewContextFrom(svnurl string,
	Verbose       bool,
	Metadata      bool,   // include metadata in git logs (git-svn-id)
	NoMinimizeUrl bool,   // accept URLs as-is without attempting to connect a higher level directory
	RootIsTrunk   bool,   // use this if the root level of the repo is equivalent to the trunk and there are no tags or branches
	Rebase        bool,   // instead of cloning a new project, rebase an existing one against SVN
	UserName      string, // username for transports that needs it (http(s), svn)
	Trunk         string, // subpath to trunk from repository URL
	Branches      string, // subpath to branches from repository URL
	Tags          string, // subpath to tags from repository URL
	Exclude       string, // regular expression to filter paths when fetching
	Revision      string, // start importing from SVN revision START_REV; optionally end at END_REV. e.g. START_REV:END_REV

	NoTrunk    bool,   // do not import anything from trunk
	NoBranches bool,   // do not import anything from branches
	NoTags     bool,   // do not import anything from tags
	Authors    string, // path to file containing svn-to-git authors mapping
) *Context {
	ctx := &Context{
		Repo: Repo{
			local_branches:  []string{},
			remote_branches: []string{},
			tags:            []string{},
		},
		Url:           svnurl,
		Verbose:       Verbose,
		Metadata:      Metadata,
		NoMinimizeUrl: NoMinimizeUrl,
		RootIsTrunk:   RootIsTrunk,
		Rebase:        Rebase,
		UserName:      UserName,
		Trunk:         Trunk,
		Branches:      Branches,
		Tags:          Tags,
		Exclude:       Exclude,
		Revision:      Revision,
		NoTrunk:       NoTrunk,
		NoBranches:    NoBranches,
		NoTags:        NoTags,
		Authors:       os.ExpandEnv(Authors),
	}
	if !path_exists(ctx.Authors) {
		ctx.Authors = ""
	}
	return ctx
}

func (ctx *Context) print_cmd(cmd *exec.Cmd) {
	if ctx.Verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
	}
}

func (ctx *Context) debug_cmd(cmd *exec.Cmd) {
	if ctx.Verbose {
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
}

func (ctx *Context) git_cmd(cmdargs ...string) []string {
	lines := []string{}
	cmd := exec.Command("git", cmdargs...)
	if ctx.Verbose {
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

func (ctx *Context) Run() error {
	var err error
	if ctx.Rebase {
		err = ctx.get_branches()
	} else {
		err = ctx.do_clone()
	}
	if err != nil {
		return err
	}

	err = ctx.fix_tags()
	if err != nil {
		return err
	}

	err = ctx.fix_branches()
	if err != nil {
		return err
	}

	err = ctx.fix_trunk()
	if err != nil {
		return err
	}

	err = ctx.optimize_repos()
	if err != nil {
		return err
	}

	return err
}

func (ctx *Context) get_branches() error {
	var err error = nil
	// get the list of local and remote branches
	// ignore console color codes
	cmd := exec.Command("git", "branch", "-l", "--no-color")
	if ctx.Verbose {
		fmt.Printf(":: --> building list of [local branches]...\n")
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}
	lines := bufio.NewReader(bytes.NewBuffer(out))
	for line := range iochan.ReaderChan(lines, "\n") {
		if ctx.Verbose {
			fmt.Printf("   %q\n", line)
		}
		if strings.HasPrefix(line, "*") {
			line = strings.Replace(line, "*", "", 1)
		}
		line = strings.Trim(line, " \r\n")
		if ctx.Verbose {
			fmt.Printf("-> %q\n", line)
		}
		ctx.Repo.local_branches = append(ctx.Repo.local_branches, line)
	}

	// remote branches...
	cmd = exec.Command("git", "branch", "-r", "--no-color")
	if ctx.Verbose {
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
		if ctx.Verbose {
			fmt.Printf("   %q\n", line)
		}
		if strings.HasPrefix(line, "*") {
			line = strings.Replace(line, "*", "", 1)
		}
		line = strings.Trim(line, " \r\n")
		if ctx.Verbose {
			fmt.Printf("-> %q\n", line)
		}
		ctx.Repo.remote_branches = append(ctx.Repo.remote_branches, line)
	}

	// tags are remote branches that start with "svn/tags/"
	if ctx.Verbose {
		fmt.Printf(":: --> building list of [svn-tags]...\n")
	}
	for _, branch := range ctx.Repo.remote_branches {
		if strings.HasPrefix(branch, "svn/tags/") {
			tag := branch //branch[len("svn/tags/"):]
			if ctx.Verbose {
				fmt.Printf(":: adding tag %q...\n", tag)
			}
			ctx.Repo.tags = append(ctx.Repo.tags, tag)

		}
	}
	return err
}

func (ctx *Context) do_clone() error {
	var err error = nil

	cmdargs := []string{
		"svn", "init", "--prefix=svn/",
	}
	var cmd *exec.Cmd = nil
	if ctx.RootIsTrunk {
		// non-standard repository layout.
		// The repository root is effectively 'trunk.'
		if ctx.UserName != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--username=%s", ctx.UserName))
		}
		if ctx.Metadata {
			cmdargs = append(cmdargs, "--no-metadata")
		}
		if ctx.NoMinimizeUrl {
			cmdargs = append(cmdargs, "--no-minimize-url")
		}
		cmdargs = append(cmdargs, fmt.Sprintf("--trunk=%s", ctx.Url))
	} else {
		// add each component to the command that was passed as an argument
		if ctx.UserName != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--username=%s", ctx.UserName))
		}
		if ctx.Metadata {
			cmdargs = append(cmdargs, "--no-metadata")
		}
		if ctx.NoMinimizeUrl {
			cmdargs = append(cmdargs, "--no-minimize-url")
		}
		if ctx.Trunk != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--trunk=%s", ctx.Trunk))
		}
		if ctx.Tags != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--tags=%s", ctx.Tags))
		}
		if ctx.Branches != "" {
			cmdargs = append(cmdargs, fmt.Sprintf("--branches=%s", ctx.Branches))
		}
		cmdargs = append(cmdargs, ctx.Url)
	}
	cmd = exec.Command("git", cmdargs...)
	if ctx.Verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	err = cmd.Run()
	if err != nil {
		return err
	}

	if ctx.Authors != "" {
		cmd := exec.Command("git", "config", "--local", "svn.authorsfile",
			ctx.Authors)

		if ctx.Verbose {
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
		if ctx.Verbose {
			fmt.Printf("no authors file...\n")
		}
	}

	cmdargs = []string{"svn", "fetch"}
	if ctx.Revision != "" {
		rev := strings.Split(ctx.Revision, ":")
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
			return fmt.Errorf("invalid argument to '-revision' flag (%q)", ctx.Revision)
		}
		cmdargs = append(cmdargs,
			"-r",
			fmt.Sprintf("%s:%s", rev[0], rev[1]),
		)
	}
	if ctx.Exclude != "" {
		patterns := []string{}
		if ctx.RootIsTrunk {
			if ctx.Trunk != "" {
				patterns = append(patterns, ctx.Trunk+"[/]")
			}
			if ctx.Tags != "" {
				patterns = append(patterns, ctx.Tags+"[/][^/]+[/]")
			}
			if ctx.Branches != "" {
				patterns = append(patterns, ctx.Branches+"[/][^/]+[/]")
			}
		}
		regex := fmt.Sprintf("^(?:%s)(?:%s)",
			strings.Join(patterns, "|"),
			ctx.Exclude)
		cmdargs = append(cmdargs,
			fmt.Sprintf("--ignore-paths=\"%s\"", regex),
		)
	}

	cmd = exec.Command("git", cmdargs...)
	if ctx.Verbose {
		fmt.Printf(":: running %s\n", strings.Join(cmd.Args, " "))
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}

	err = cmd.Run()
	if err != nil {
		return err
	}

	err = ctx.get_branches()
	if err != nil {
		return err
	}

	return err
}

func (ctx *Context) fix_tags() error {
	var err error = nil
	usr := make(map[string]string)

	// we only change git config values if ctx.Repo.tags are available.
	// so it stands to reason we should revert them only in that case.
	defer func() {
		if len(ctx.Repo.tags) == 0 {
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

	for itag, tag := range ctx.Repo.tags {
		tag = strings.Trim(tag, " ")
		id := tag[len("svn/tags/"):]
		if ctx.Verbose {
			hdr := ""
			if itag > 0 {
				hdr = "\n"
			}
			fmt.Printf("%s:: processing svn tag [%s]...\n", hdr, tag)
		}
		subject := ctx.git_cmd("log", "-1", "--pretty=format:%s", tag)[0]
		date := ctx.git_cmd("log", "-1", "--pretty=format:%ci", tag)[0]
		author := ctx.git_cmd("log", "-1", "--pretty=format:%an", tag)[0]
		email := ctx.git_cmd("log", "-1", "--pretty=format:%ae", tag)[0]

		cmd := exec.Command("git", "config", "--local", "user.name",
			"\""+author+"\"")
		ctx.print_cmd(cmd)
		_ = cmd.Run()

		cmd = exec.Command("git", "config", "--local", "user.email",
			"\""+email+"\"")
		ctx.print_cmd(cmd)
		_ = cmd.Run()

		cmd = exec.Command("git", "tag", "-a", "-m",
			fmt.Sprintf("\"%s\"", subject),
			id,
			tag)
		cmd.Env = os.Environ()
		cmd.Env = append(cmd.Env, fmt.Sprintf("GIT_COMMITTER_DATE=%s", date))
		ctx.print_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			fmt.Printf("**error** %v\n", err)
			return err
		}

		cmd = exec.Command("git", "branch", "-d", "-r", tag)
		ctx.print_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			fmt.Printf("**error** %v\n", err)
			return err
		}

		//fmt.Printf("tag: %q - subject: %q\n", tag, subject)
	}
	return err
}

func (ctx *Context) fix_branches() error {
	var err error = nil
	svn_branches := []string{}
	for _, v := range ctx.Repo.remote_branches {
		if is_in_slice(v, ctx.Repo.tags) {
			if ctx.Verbose {
				fmt.Printf("-- discard [%s]...\n", v)
			}
			continue
		}
		if strings.HasPrefix(v, "svn/") {
			svn_branches = append(svn_branches, v)
		}
	}
	if ctx.Verbose {
		fmt.Printf("svn-branches: %v\n", svn_branches)
	}

	if ctx.Rebase {
		cmd := exec.Command("git", "svn", "fetch")
		ctx.print_cmd(cmd)
		if ctx.Verbose {
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
		if ctx.Rebase && (is_in_slice(branch, ctx.Repo.local_branches) || branch == "trunk") {
			lbranch := branch
			if branch == "trunk" {
				lbranch = "master"
			}
			cmd := exec.Command("git", "checkout", "-f", lbranch)
			ctx.print_cmd(cmd)
			ctx.debug_cmd(cmd)
			err = cmd.Run()
			if err != nil {
				return err
			}

			cmd = exec.Command("git", "rebase",
				fmt.Sprintf("remotes/svn/%s", branch),
			)
			ctx.print_cmd(cmd)
			ctx.debug_cmd(cmd)
			err = cmd.Run()
			if err != nil {
				return err
			}
			continue
		}

		if branch == "trunk" || is_in_slice(branch, ctx.Repo.local_branches) {
			continue
		}

		cmd := exec.Command("git", "branch", branch,
			fmt.Sprintf("remotes/svn/%s", branch))
		ctx.print_cmd(cmd)
		ctx.debug_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			return err
		}

		cmd = exec.Command("git", "checkout", branch)
		ctx.print_cmd(cmd)
		ctx.debug_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	return err
}

func (ctx *Context) fix_trunk() error {
	var err error = nil
	trunk := ""
	for _, v := range ctx.Repo.remote_branches {
		if strings.Trim(v, " ") == "trunk" {
			trunk = "trunk"
			break
		}
	}
	var cmds []string
	if trunk != "" && !ctx.Rebase {
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
		ctx.print_cmd(cmd)
		ctx.debug_cmd(cmd)
		err = cmd.Run()
		if err != nil {
			return err
		}
	}
	return err
}

func (ctx *Context) optimize_repos() error {
	var err error = nil
	cmd := exec.Command("git", "gc")
	ctx.print_cmd(cmd)
	ctx.debug_cmd(cmd)
	err = cmd.Run()
	return err
}

func (ctx *Context) verify_working_tree_is_clean() error {
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=no")
	out, err := cmd.CombinedOutput()
	if len(out) != 0 {
		fmt.Printf("** you have pending changes. The working tree must be clean in order to continue.\n")
	}
	return err
}

// EOF
