go-svn2git
==========

`go-svn2git` is a tiny utility for migrating projects from Subversion
to Git while keeping the trunk, branches and tags where they should
be. It uses git-svn to clone an svn repository and does some clean-up
to make sure branches and tags are imported in a meaningful way, and
that the code checked into master ends up being what's currently in
your svn trunk rather than whichever svn branch your last commit was
in.

This is actually a re-write of
[svn2git](https://github.com/nirvdrum/svn2git) in `Go`.




Examples
--------

Say I have this code in svn:

    trunk
      ...
    branches
      1.x
      2.x
    tags
      1.0.0
      1.0.1
      1.0.2
      1.1.0
      2.0.0

git-svn will go through the commit history to build a new git repo. It will
import all branches and tags as remote svn branches, whereas what you really
want is git-native local branches and git tag objects. So after importing this
project I'll get:

    $ git branch
    * master
    $ git branch -a
    * master
      1.x
      2.x
      tags/1.0.0
      tags/1.0.1
      tags/1.0.2
      tags/1.1.0
      tags/2.0.0
      trunk
    $ git tag -l
    [ empty ]

After `go-svn2git` is done with your project, you'll get this instead:

    $ git branch
    * master
      1.x
      2.x
    $ git tag -l
      1.0.0
      1.0.1
      1.0.2
      1.1.0
      2.0.0

Finally, it makes sure the HEAD of master is the same as the current trunk of
the svn repo.

Installation
------------

Make sure you have `git`, `git-svn`, and `go` installed.  
`go-svn2git` is a `go` wrapper around git's native SVN support through
git-svn.  It is possible to have git installed without git-svn
installed, so please do verify that you can run `$ git svn`
successfully.


Usage
-----

### Initial Conversion ###

There are several ways you can create a git repo from an existing
svn repo. The differentiating factor is the svn repo layout. Below is an
enumerated listing of the varying supported layouts and the proper way to
create a git repo from a svn repo in the specified layout.

1. The svn repo is in the standard layout of (trunk, branches, tags) at the
root level of the repo.

        $ go-svn2git http://svn.example.com/path/to/repo

2. The svn repo is NOT in standard layout and has only a trunk and tags at the
root level of the repo.

        $ go-svn2git http://svn.example.com/path/to/repo -trunk dev -tags rel -no-branches

3. The svn repo is NOT in standard layout and has only a trunk at the root
level of the repo.

        $ go-svn2git http://svn.example.com/path/to/repo -trunk trunk -no-branches -no-tags

4. The svn repo is NOT in standard layout and has no trunk, branches, or tags
at the root level of the repo. Instead the root level of the repo is
equivalent to the trunk and there are no tags or branches.

        $ go-svn2git http://svn.example.com/path/to/repo -root-is-trunk

5. The svn repo is in the standard layout but you want to exclude the massive
doc directory and the backup files you once accidently added.

        $ go-svn2git http://svn.example.com/path/to/repo -exclude doc -exclude '.*~$'

6. The svn repo actually tracks several projects and you only want to migrate
one of them.

        $ go-svn2git http://svn.example.com/path/to/repo/nested_project -no-minimize-url

7. The svn repo is password protected.

        $ go-svn2git http://svn.example.com/path/to/repo -username <<user_with_perms>>

8. You need to migrate starting at a specific svn revision number.

        $ go-svn2git http://svn.example.com/path/to/repo -revision <<starting_revision_number>>

9. You need to migrate starting at a specific svn revision number, ending at a specific revision number.

        $ go-svn2git http://svn.example.com/path/to/repo -revision <<starting_revision_number>>:<<ending_revision_number>>

The above will create a git repository in the current directory with the git
version of the svn repository. Hence, you need to make a directory that you
want your new git repo to exist in, change into it and then run one of the
above commands. Note that in the above cases the trunk, branches, tags options
are simply folder names relative to the provided repo path. For example if you
specified trunk=foo branches=bar and tags=foobar it would be referencing
http://svn.example.com/path/to/repo/foo as your trunk, and so on. However, in
case 4 it references the root of the repo as trunk.

### Repository Updates ###

As of go-svn2git 2.0 there is a new feature to pull in the latest changes from SVN into your
git repository created with go-svn2git.  This is a one way sync, but allows you to use svn2git
as a mirroring tool for your SVN repositories.

The command to call is:

        $ cd <EXISTING_REPO> && go-svn2git -rebase

Authors
-------

To convert all your svn authors to git format, create a file somewhere on your
system with the list of conversions to make, one per line, for example:

    jcoglan = James Coglan <jcoglan@never-you-mind.com>
    stnick = Santa Claus <nicholas@lapland.com>

Then pass an +authors+ option to +go-svn2git+ pointing to your file:

    $ go-svn2git http://svn.example.com/path/to/repo -authors ~/authors.txt

Alternatively, you can place the authors file into ~/.go-svn2git/authors and
svn2git will load it out of there. This allows you to build up one authors
file for all your projects and have it loaded for each repository that you
migrate.

If you need a jump start on figuring out what users made changes in your
svn repositories the following command sequence might help. It grabs all
the logs from the svn repository, pulls out all the names from the commits,
sorts them, and then reduces the list to only unique names. So, in the end
it outputs a list of usernames of the people that made commits to the svn
repository which name on its own line. This would allow you to easily
redirect the output of this command sequence to ~/.config/go-svn2git/authors and have
a very good starting point for your mapping.

    $ svn log | grep -E "r[0-9]+ \| .+ \|" | awk '{print $3}' | sort | uniq
