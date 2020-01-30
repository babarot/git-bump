package main

import (
	"errors"
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
}

func (c CLI) Run(args []string) error {
	path := args[0]

	r, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	latest, err := getCurrentVersion(r)
	if err != nil {
		return err
	}

	prompt := promptui.Select{
		Label: "Select Day",
		Items: []string{"patch", "minor", "major"},
	}
	_, result, err := prompt.Run()
	if err != nil {
		return err
	}

	var next semver.Version
	switch result {
	case "major":
		next = latest.IncMajor()
	case "minor":
		next = latest.IncMinor()
	case "patch":
		next = latest.IncPatch()
	default:
		return errors.New("no")
	}

	head, err := r.Head()
	if err != nil {
		return err
	}

	commit, err := r.CommitObject(head.Hash())
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
	v := "v" + next.String()
	_, err = r.CreateTag(v, head.Hash(), opts)
	if err != nil {
		return err
	}

	rs := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", v, v))
	// rs := config.RefSpec("refs/tags/*:refs/tags/*")

	if err := r.Push(&git.PushOptions{
		Auth: &http.BasicAuth{
			Username: user,
			Password: os.Getenv("GITHUB_TOKEN"),
		},
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{rs},
		Progress:   os.Stdout,
	}); err != nil {
		return err
	}

	return nil
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func getCurrentVersion(r *git.Repository) (*semver.Version, error) {
	var latest *semver.Version

	tagrefs, err := r.Tags()
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
