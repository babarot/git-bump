package main

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

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

const (
	Prefix string = "v"
)

type Spec int

const (
	Major Spec = iota
	Minor
	Patch
)

func (c Spec) String() string {
	switch c {
	case Major:
		return "major"
	case Minor:
		return "minor"
	case Patch:
		return "patch"
	default:
		return "unknown"
	}
}

type Option struct {
	Major bool `long:"major" description:"Bump up major version"`
	Minor bool `long:"minor" description:"Bump up minor version"`
	Patch bool `long:"patch" description:"Bump up patch version"`
}

type CLI struct {
	Option Option
	Repo   *git.Repository
}

func main() {
	os.Exit(run(os.Args[1:]))
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

func (c *CLI) Run(args []string) error {
	var wd string
	switch len(args) {
	case 0:
		wd = "."
	default:
		wd = args[0]
	}

	r, err := git.PlainOpen(wd)
	if err != nil {
		return err
	}
	c.Repo = r

	current, err := c.currentVersion()
	if err != nil {
		return err
	}

	next, err := c.nextVersion(current)
	if err != nil {
		return err
	}

	tag := next.String()
	if strings.HasPrefix(current.Original(), Prefix) {
		tag = Prefix + next.String()
	}

	return c.PushTag(tag)
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
	var current *semver.Version

	tagrefs, err := c.Repo.Tags()
	if err != nil {
		return current, err
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
		return current, err
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
	current = vs[len(vs)-1]

	for _, v := range vs {
		fmt.Printf("%s\n", v.Original())
	}

	return current, nil
}

func (c *CLI) prompt(label string, items []Spec) (Spec, error) {
	prompt := promptui.Select{
		Label:        label,
		Items:        items,
		HideSelected: true,
	}
	i, _, err := prompt.Run()
	return items[i], err
}

func (c *CLI) nextVersion(current *semver.Version) (semver.Version, error) {
	var next semver.Version

	specs := []Spec{}
	if c.Option.Major {
		specs = append(specs, Major)
	}
	if c.Option.Minor {
		specs = append(specs, Minor)
	}
	if c.Option.Patch {
		specs = append(specs, Patch)
	}

	label := fmt.Sprintf("Current tag is %q. Next is?", current.Original())

	var spec Spec
	switch len(specs) {
	case 0:
		// No flags specified
		spec, _ = c.prompt(label, []Spec{Patch, Minor, Major})
	case 1:
		// One flag
		spec = specs[0]
	default:
		// Multiple: e.g. --major --patch
		spec, _ = c.prompt(label, specs)
	}

	switch spec {
	case Major:
		next = current.IncMajor()
	case Minor:
		next = current.IncMinor()
	case Patch:
		next = current.IncPatch()
	default:
		return next, errors.New("invalid semver")
	}

	return next, nil
}
