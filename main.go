package main

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
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
	"gopkg.in/src-d/go-git.v4/plumbing/transport/ssh"
)

// These variables are set in build step
var (
	Version  = "unset"
	Revision = "unset"
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

type CLI struct {
	Option Option
	Stdout io.Writer
	Stderr io.Writer
	Repo   *git.Repository

	initial *semver.Version
}

type Option struct {
	Major bool `long:"major" description:"Bump up major version"`
	Minor bool `long:"minor" description:"Bump up minor version"`
	Patch bool `long:"patch" description:"Bump up patch version"`

	Quiet bool `short:"q" long:"quiet" description:"Be quiet"`
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
	cli := CLI{
		Option: opt,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Repo:   nil,
	}
	if err := cli.Run(args); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		return 1
	}
	return 0
}

func (c *CLI) Run(args []string) error {
	if c.Option.Quiet {
		c.Stdout, c.Stderr = ioutil.Discard, ioutil.Discard
	}

	var wd string
	switch len(args) {
	case 0:
		wd = "."
	default:
		wd = args[0]
	}

	r, err := git.PlainOpen(wd)
	if err != nil {
		if errors.Is(err, git.ErrRepositoryNotExists) {
			// make error message git-like
			return errors.New("fatal: not a git repository")
		}
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
		// SignKey: TODO: set gpg sign key
	}

	_, err = c.Repo.CreateTag(tag, head.Hash(), opts)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.Stdout, "Bump version to %q.\n", tag)

	// TODO: set if all tags
	// rs := config.RefSpec("refs/tags/*:refs/tags/*")
	rs := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", tag, tag))

	defer fmt.Fprintf(c.Stdout, "Pushed to origin.\n")

	pushOptions := &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{rs},
		Progress:   c.Stdout,
	}

	if sshKey, ok := os.LookupEnv("SSH_KEY"); ok {
		var publicKey *ssh.PublicKeys
		sshKeyContents, err := ioutil.ReadFile(sshKey)
		if err != nil {
			fmt.Fprintf(c.Stderr, "Unable to read SSH_KEY: %s\n", err)
		}
		publicKey, err = ssh.NewPublicKeys("git", []byte(sshKeyContents), "")
		if err != nil {
			fmt.Fprintf(c.Stderr, "Unable to parse SSH_KEY: %s\n", err)
		}
		pushOptions.Auth = publicKey
	}

	if githubToken, ok := os.LookupEnv("GITHUB_TOKEN"); ok {
		pushOptions.Auth = &http.BasicAuth{
			Username: user,
			Password: githubToken,
		}
	}

	return c.Repo.Push(pushOptions)
}

func (c *CLI) newVersion() (*semver.Version, error) {
	validate := func(input string) error {
		_, err := semver.NewVersion(input)
		return err
	}

	prompt := promptui.Prompt{
		Label:    "New version",
		Validate: validate,
	}

	v, err := prompt.Run()
	if err != nil {
		return nil, err
	}

	return semver.NewVersion(v)
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

	// No tags found
	if len(tags) == 0 {
		v, err := c.newVersion()
		if err != nil {
			return current, fmt.Errorf("%w: cannot create new version", err)
		}
		c.initial = v
		return v, nil
	}

	vs := make([]*semver.Version, 0)
	for _, tag := range tags {
		v, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}
		vs = append(vs, v)
	}

	sort.Sort(semver.Collection(vs))

	last := len(vs) - 1
	current = vs[last]

	fmt.Fprintln(c.Stdout, "Tags:")
	for i, v := range vs {
		msg := fmt.Sprintf("- %s", v.Original())
		if i == last {
			msg = fmt.Sprintf("- %s (current version)", v.Original())
		}
		fmt.Fprintln(c.Stdout, msg)
	}
	fmt.Fprintln(c.Stdout)

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

	if c.initial != nil {
		return *c.initial, nil
	}

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
	var err error
	switch len(specs) {
	case 0:
		// No flags specified
		spec, err = c.prompt(label, []Spec{Patch, Minor, Major})
		if err != nil {
			return next, fmt.Errorf("%w: failed to select valid semver spec", err)
		}
	case 1:
		// One flag
		spec = specs[0]
	default:
		// Multiple: e.g. --major --patch
		spec, err = c.prompt(label, specs)
		if err != nil {
			return next, fmt.Errorf("%w: failed to select valid semver spec", err)
		}
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
