package main

import (
	"io/ioutil"
	"testing"
)

type testPair struct {
	tags        []string
	nextVersion string
}

var tests = []testPair{
	{[]string{"0.0.1"}, "0.0.2"},
	{[]string{"v0.0.1", "v0.0.2"}, "v0.0.3"},
	{[]string{"v0.0.1", "0.0.2"}, "0.0.3"},
}

var testsMetaIntel = []testPair{
	{[]string{"0.0.1+amd", "0.0.1+intel", "0.0.2+amd", "0.0.3+amd"}, "0.0.2+intel"},
}

var testsMetaAmd = []testPair{
	{[]string{"0.0.1+amd", "0.0.1+intel", "0.0.2+amd", "0.0.3+amd"}, "0.0.4+amd"},
}

func TestFindCurrentTag(t *testing.T) {
	createCli("").runScenarios(t, tests)
}

func TestFindCurrentTagForMeta(t *testing.T) {
	createCli("intel").runScenarios(t, testsMetaIntel)
	createCli("amd").runScenarios(t, testsMetaAmd)
}

func (c *CLI) runScenarios(t *testing.T, scenarios []testPair) {
	for _, pair := range scenarios {
		currentVersion, _ := c.findCurrentVersion(pair.tags)
		nextVersion, _ := c.createNextVersion(currentVersion)
		if nextVersion != pair.nextVersion {
			t.Error(
				"For", pair.tags,
				"expected", pair.nextVersion,
				"got", nextVersion,
			)
		}
	}
}

func createCli(meta string) *CLI {
	cli := CLI{
		Stdout: ioutil.Discard,
		Stderr: ioutil.Discard,
		Repo:   nil,
		Option: Option{Major: false, Minor: false, Patch: true, Meta: meta, Quiet: true},
	}
	return &cli
}
