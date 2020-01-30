package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/Masterminds/semver/v3"
	"github.com/Songmu/gitconfig"
	"github.com/k0kubun/pp"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/object"
	"gopkg.in/src-d/go-git.v4/plumbing/transport/http"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	path := os.Args[1]

	r, err := git.PlainOpen(path)
	if err != nil {
		return err
	}

	tagrefs, err := r.Tags()
	if err != nil {
		return err
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
		return err
	}

	vs := make([]*semver.Version, len(tags))
	for i, tag := range tags {
		v, err := semver.NewVersion(tag)
		if err != nil {
			return err
		}

		vs[i] = v
	}

	sort.Sort(semver.Collection(vs))

	latest := vs[len(vs)-1]

	which := os.Args[2]

	var next semver.Version
	switch which {
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

	user, err := gitconfig.User()
	if err != nil {
		return err
	}

	email, err := gitconfig.Email()
	if err != nil {
		return err
	}

	pp.Println(user, email)

	opts := &git.CreateTagOptions{
		Tagger: &object.Signature{
			Name:  user,
			Email: email,
			// When:  time.Now(),
		},
		Message: "",
	}
	v := "v" + next.String()
	_, err = r.CreateTag(v, head.Hash(), opts)
	if err != nil {
		return err
	}

	rs := config.RefSpec(fmt.Sprintf("refs/tags/%s:refs/tags/%s", v, v))
	// rs := config.RefSpec("refs/tags/*:refs/tags/*")

	if err := r.PushContext(context.Background(), &git.PushOptions{
		Auth: &http.BasicAuth{
			Username: user,
			Password: os.Getenv("GITHUB_TOKEN"),
		},
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{rs},
	}); err != nil {
		return err
	}

	return nil
}
