package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/Songmu/gitconfig"
	"github.com/jessevdk/go-flags"
	"github.com/manifoldco/promptui"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

type Option struct {
	Verbose []bool `short:"v" long:"verbose" description:"Show verbose debug information"`
}

type CLI struct {
	Option Option
	Repo   *git.Repository
}

func (c *CLI) Run(args []string) error {
	r, err := git.PlainOpen(args[0])
	if err != nil {
		return err
	}
	c.Repo = r

	latest, err := c.currentVersion()
	if err != nil {
		return err
	}

	next, err := c.nextVersion(latest)
	if err != nil {
		return err
	}

	tag := "v" + next.String()
	return c.PushTag(tag)
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func (c *CLI) PushTag(tag string) error {
	head, err := c.Repo.Head()
	if err != nil {
		return err
	}

	commit, err := c.Repo.CommitObject(head.Hash())
	if err != nil {
		return err
	}

	user, err := gitconfig.User()
	if err != nil {
		return err
	}

	email, err := gitconfig.Email()
	if err != nil {
		return err
	}

	opts := &git.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  user,
			Email: email,
			When:  commit.Committer.When,
		},
		Message: commit.Message,
		// SignKey:
	}

	_, err = c.Repo.CreateTag(tag, head.Hash(), opts)
	if err != nil {
		return err
	}

	rs := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", tag, tag))
	// rs := config.RefSpec("refs/tags/*:refs/tags/*")

	return c.Repo.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: user,
			Password: os.Getenv("GITHUB_TOKEN"),
		},
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{rs},
		Progress:   os.Stdout,
	})
}

func (c *CLI) currentVersion() (*semver.Version, error) {
	var latest *semver.Version

	tagrefs, err := c.Repo.Tags()
	if err != nil {
		return latest, err
	}

	var tags []string
	err = tagrefs.ForEach(func(t *plumbing.Reference) error {
		tag := t.Name()
		if tag.IsTag() {
			tags = append(tags, tag.Short())
		}
		return nil
	})
	if err != nil {
		return latest, err
	}

	vs := make([]*semver.Version, len(tags))
	for i, tag := range tags {
		v, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}
		vs[i] = v
	}

	sort.Sort(semver.Collection(vs))
	latest = vs[len(vs)-1]

	return latest, nil
}

func (c *CLI) nextVersion(latest *semver.Version) (semver.Version, error) {
	var next semver.Version

	prompt := promptui.Select{
		Label: "Select next version",
		Items: []string{"patch", "minor", "major"},
	}
	_, result, err := prompt.Run()
	if err != nil {
		return next, err
	}

	switch result {
	case "major":
		next = latest.IncMajor()
	case "minor":
		next = latest.IncMinor()
	case "patch":
		next = latest.IncPatch()
	}

	return next, nil
}

func run(args []string) int {
	var opt Option
	args, err := flags.ParseArgs(&opt, args)
	if err != nil {
		return 1
	}
	cli := CLI{Option: opt}
	if err := cli.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		return 1
	}
	return 0
}
