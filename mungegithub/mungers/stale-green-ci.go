/*
Copyright 2015 The Kubernetes Authors All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mungers

import (
	"fmt"
	"time"

	"k8s.io/contrib/mungegithub/features"
	"k8s.io/contrib/mungegithub/github"

	"github.com/golang/glog"
	githubapi "github.com/google/go-github/github"
	"github.com/spf13/cobra"
)

const (
	staleGreenCIHours = 96
	greenMsgFormat    = `@` + jenkinsBotName + ` test this

Tests are more than %d hours old. Re-running tests.`
)

var (
	greenMsgBody     = fmt.Sprintf(greenMsgFormat, staleGreenCIHours)
	requiredContexts = []string{jenkinsUnitContext, jenkinsE2EContext}
)

// StaleGreenCI will remove the LGTM flag from an PR which has been
// updated since the reviewer added LGTM
type StaleGreenCI struct{}

func init() {
	s := StaleGreenCI{}
	RegisterMungerOrDie(s)
	RegisterStaleComments(s)
}

// Name is the name usable in --pr-mungers
func (StaleGreenCI) Name() string { return "stale-green-ci" }

// RequiredFeatures is a slice of 'features' that must be provided
func (StaleGreenCI) RequiredFeatures() []string { return []string{} }

// Initialize will initialize the munger
func (StaleGreenCI) Initialize(config *github.Config, features *features.Features) error {
	return nil
}

// EachLoop is called at the start of every munge loop
func (StaleGreenCI) EachLoop() error { return nil }

// AddFlags will add any request flags to the cobra `cmd`
func (StaleGreenCI) AddFlags(cmd *cobra.Command, config *github.Config) {}

// Munge is the workhorse the will actually make updates to the PR
func (StaleGreenCI) Munge(obj *github.MungeObject) {
	if !obj.IsPR() {
		return
	}

	if !obj.HasLabel(lgtmLabel) {
		return
	}

	if mergeable, err := obj.IsMergeable(); !mergeable || err != nil {
		return
	}

	if !obj.IsStatusSuccess(requiredContexts) {
		return
	}

	for _, context := range requiredContexts {
		statusTime := obj.GetStatusTime(context)
		if statusTime == nil {
			glog.Errorf("%d: unable to determine time %q context was set", *obj.Issue.Number, context)
			return
		}
		if time.Since(*statusTime) > staleGreenCIHours*time.Hour {
			obj.WriteComment(greenMsgBody)
			err := obj.WaitForPending(requiredContexts)
			if err != nil {
				glog.Errorf("Failed waiting for PR to start testing: %v", err)
			}
			return
		}
	}
}

func (StaleGreenCI) isStaleComment(obj *github.MungeObject, comment githubapi.IssueComment) bool {
	if !mergeBotComment(comment) {
		return false
	}
	if *comment.Body != greenMsgBody {
		return false
	}
	stale := commentBeforeLastCI(obj, comment)
	if stale {
		glog.V(6).Infof("Found stale StaleGreenCI comment")
	}
	return stale
}

// StaleComments returns a slice of stale comments
func (s StaleGreenCI) StaleComments(obj *github.MungeObject, comments []githubapi.IssueComment) []githubapi.IssueComment {
	return forEachCommentTest(obj, comments, s.isStaleComment)
}

func commentBeforeLastCI(obj *github.MungeObject, comment githubapi.IssueComment) bool {
	if !obj.IsStatusSuccess(requiredContexts) {
		return false
	}
	if comment.CreatedAt == nil {
		return false
	}
	commentTime := *comment.CreatedAt

	for _, context := range requiredContexts {
		statusTimeP := obj.GetStatusTime(context)
		if statusTimeP == nil {
			return false
		}
		statusTime := statusTimeP.Add(30 * time.Minute)
		if commentTime.After(statusTime) {
			return false
		}
	}
	return true
}
