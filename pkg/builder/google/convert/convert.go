/*
Copyright 2018 Google, Inc. All rights reserved.

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
package convert

import (
	"encoding/json"
	"fmt"
	"reflect"

	"google.golang.org/api/cloudbuild/v1"
	corev1 "k8s.io/api/core/v1"

	v1alpha1 "github.com/google/build-crd/pkg/apis/cloudbuild/v1alpha1"
)

const (
	customSource = "custom-source"
)

var (
	emptyVolumeSource = corev1.VolumeSource{
		EmptyDir: &corev1.EmptyDirVolumeSource{},
	}
)

func remarshal(in, out interface{}) error {
	bts, err := json.Marshal(in)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(bts, &out); err != nil {
		return err
	}
	return nil
}

func FromCRD(u *v1alpha1.BuildSpec) (*cloudbuild.Build, error) {
	bld := cloudbuild.Build{
		Steps: make([]*cloudbuild.BuildStep, 0, len(u.Steps)),
	}
	if u.Source != nil {
		switch {
		case u.Source.Git != nil:
			scm, err := ToRepoSourceFromGit(u.Source.Git)
			if err != nil {
				return nil, err
			}
			bld.Source = &cloudbuild.Source{
				RepoSource: scm,
			}

		case u.Source.Custom != nil:
			step, err := ToStepFromContainer(u.Source.Custom)
			if err != nil {
				return nil, err
			}
			step.Id = customSource
			bld.Steps = append(bld.Steps, step)

		default:
			return nil, fmt.Errorf("unsupported Source, got %v", u.Source)
		}
	}
	for _, c := range u.Steps {
		step, err := ToStepFromContainer(&c)
		if err != nil {
			return nil, err
		}
		bld.Steps = append(bld.Steps, step)
	}
	// We only support emptyDir volumes.
	for _, v := range u.Volumes {
		if !reflect.DeepEqual(v.VolumeSource, emptyVolumeSource) {
			return nil, fmt.Errorf("only emptyDir volumes are supported, got %v", v.VolumeSource)
		}
	}
	return &bld, nil
}

func ToCRD(u *cloudbuild.Build) (*v1alpha1.BuildSpec, error) {
	bld := v1alpha1.BuildSpec{
		Steps: make([]corev1.Container, 0, len(u.Steps)),
	}
	steps := u.Steps
	switch {
	case u.Source != nil && u.Source.RepoSource != nil:
		scm, err := ToGitFromRepoSource(u.Source.RepoSource)
		if err != nil {
			return nil, err
		}
		bld.Source = &v1alpha1.SourceSpec{
			Git: scm,
		}
	case steps[0].Id == customSource:
		c, err := ToContainerFromStep(steps[0])
		if err != nil {
			return nil, err
		}
		c.Name = ""
		steps = steps[1:]
		bld.Source = &v1alpha1.SourceSpec{
			Custom: c,
		}
	case u.Source != nil:
		return nil, fmt.Errorf("unsupported Source, got: %v", u.Source)
	}
	volumeNames := make(map[string]bool)
	for _, step := range steps {
		c, err := ToContainerFromStep(step)
		if err != nil {
			return nil, err
		}
		for _, v := range c.VolumeMounts {
			volumeNames[v.Name] = true
		}
		bld.Steps = append(bld.Steps, *c)
	}

	// Create emptyDir volume entries, which is all GCB supports.
	for k, _ := range volumeNames {
		bld.Volumes = append(bld.Volumes, corev1.Volume{
			Name:         k,
			VolumeSource: emptyVolumeSource,
		})
	}

	return &bld, nil
}