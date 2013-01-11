package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/sbinet/go-svn2git/svn"
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

func git_svn_usage() {
	fmt.Fprintf(os.Stderr, "Usage of %s:\n", os.Args[0])
	fmt.Fprintf(os.Stderr, " %s [options] SVN_URL\n", os.Args[0])
	flag.PrintDefaults()
}

func main() {
	flag.Parse()

	if *g_help {
		git_svn_usage()
		os.Exit(1)
	}

	ctx := svn.NewContextFrom(
		g_url,
		*g_verbose,
		*g_metadata,
		*g_no_minimize_url,
		*g_root_is_trunk,
		*g_rebase,
		*g_username,
		*g_trunk,
		*g_branches,
		*g_tags,
		*g_exclude,
		*g_revision,
		*g_no_trunk,
		*g_no_branches,
		*g_no_tags,
		*g_authors,
		)

	if ctx.RootIsTrunk {
		ctx.Trunk = ""
		ctx.Branches = ""
		ctx.Tags = ""
	}

	if ctx.NoTrunk {
		ctx.Trunk = ""
	}

	if ctx.NoBranches {
		ctx.Branches = ""
	}

	if ctx.NoTags {
		ctx.Tags = ""
	}

	if ctx.Verbose {
		fmt.Printf("==go-svn2git...\n")
		fmt.Printf(" verbose:  %v\n", ctx.Verbose)
		fmt.Printf(" rebase:   %v\n", ctx.Rebase)
		fmt.Printf(" username: %q\n", ctx.UserName)
		fmt.Printf(" trunk:    %q\n", ctx.Trunk)
		fmt.Printf(" branches: %q\n", ctx.Branches)
		fmt.Printf(" tags:     %q\n", ctx.Tags)
		fmt.Printf(" authors:  %q\n", ctx.Authors)
		fmt.Printf(" root-is-trunk: %v\n", ctx.RootIsTrunk)
		fmt.Printf(" exclude:  %q\n", ctx.Exclude)
	}
	
	if ctx.Rebase {
		if flag.NArg() > 0 {
			fmt.Printf("** too many arguments\n")
			fmt.Printf("** \"%s -rebase\" takes no argument\n", os.Args[0])
			//git_svn_usage()
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
			fmt.Printf("** too many arguments: %v\n", flag.Args())
			fmt.Printf("** did you pass an option *after* the url ?\n")
			ok = false
		}
		if !ok {
			fmt.Printf("** run \"%s -help\" for help\n", os.Args[0])
			os.Exit(1)
		}
		ctx.Url = flag.Arg(0)
	}

	err := ctx.Run()
	if err != nil {
		fmt.Printf("**error** %v\n", err)
		os.Exit(1)
	}
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
