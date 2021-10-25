package main

import (
	"io/ioutil"
	"testing"
)

type testpair struct {
	tags        []string
	nextVersion string
}

var tests = []testpair{
	{[]string{"v0.0.1", "v0.0.2"}, "v0.0.3"},
}

var testsMeta = []testpair{
	{[]string{"0.0.1+amd", "0.0.1+intel", "0.0.2+amd", "0.0.3+amd"}, "0.0.2+intel"},
}

func TestFindCurrentTag(t *testing.T) {
	runScenarios(t, createCli(""), tests)
}

func TestFindCurrentTagForMeta(t *testing.T) {
	runScenarios(t, createCli("intel"), testsMeta)
}

func runScenarios(t *testing.T, cli CLI, scenarios []testpair) {
	for _, pair := range scenarios {
		currentVersion, _ := findCurrentVersion(&cli, pair.tags)
		nextVersion, _ := cli.createNextVersion(currentVersion)
		if nextVersion != pair.nextVersion {
			t.Error(
				"For", pair.tags,
				"expected", pair.nextVersion,
				"got", nextVersion,
			)
		}
	}
}

func createCli(meta string) CLI {
	cli := CLI{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Repo:   nil,
		Option: Option{Major: false, Minor: false, Patch: true, Meta: meta, Quiet: true},
	}
	return cli
}
