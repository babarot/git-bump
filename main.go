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

type Option struct {
	Major bool `long:"major" description:"Major"`
	Minor bool `long:"minor" description:"Minor"`
	Patch bool `long:"patch" description:"Patch"`
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

	latest, err := c.currentVersion()
	if err != nil {
		return err
	}

	next, err := c.nextVersion(latest)
	if err != nil {
		return err
	}

	tag := next.String()
	if strings.HasPrefix(latest.Original(), "v") {
		tag = "v" + next.String()
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

	for _, v := range vs {
		fmt.Printf("%s\n", v.Original())
	}

	return latest, nil
}

func (c *CLI) prompt(label string, items []string) (string, error) {
	prompt := promptui.Select{
		Label:        label,
		Items:        items,
		HideSelected: true,
	}
	_, result, err := prompt.Run()
	return result, err
}

func (c *CLI) nextVersion(latest *semver.Version) (semver.Version, error) {
	var next semver.Version

	defaultSpecs := []string{"patch", "minor", "major"}
	specs := []string{}
	if c.Option.Major {
		specs = append(specs, "major")
	}
	if c.Option.Minor {
		specs = append(specs, "minor")
	}
	if c.Option.Patch {
		specs = append(specs, "patch")
	}

	label := fmt.Sprintf("Current version is %q. Next is?", latest.Original())

	var spec string
	switch len(specs) {
	case 0:
		spec, _ = c.prompt(label, defaultSpecs)
	case 1:
		spec = specs[0]
	default:
		spec, _ = c.prompt(label, specs)
	}

	switch spec {
	case "major":
		next = latest.IncMajor()
	case "minor":
		next = latest.IncMinor()
	case "patch":
		next = latest.IncPatch()
	default:
		return next, errors.New("invalid semver")
	}

	return next, nil
}
