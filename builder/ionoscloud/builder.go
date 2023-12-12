// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ionoscloud

import (
	"context"
	"fmt"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	"github.com/hashicorp/packer-plugin-sdk/multistep"
	"github.com/hashicorp/packer-plugin-sdk/multistep/commonsteps"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	ionoscloud "github.com/ionos-cloud/sdk-go/v6"
)

const BuilderId = "ionoscloud.builder"

type Builder struct {
	config Config
	runner multistep.Runner
}

func (b *Builder) ConfigSpec() hcldec.ObjectSpec { return b.config.FlatMapstructure().HCL2Spec() }

func (b *Builder) Prepare(raws ...interface{}) ([]string, []string, error) {
	warnings, errs := b.config.Prepare(raws...)
	if errs != nil {
		return nil, warnings, errs
	}

	return nil, warnings, nil
}

func (b *Builder) Run(ctx context.Context, ui packersdk.Ui, hook packersdk.Hook) (packersdk.Artifact, error) {
	state := new(multistep.BasicStateBag)

	state.Put("config", &b.config)
	state.Put("hook", hook)
	state.Put("ui", ui)

	client, err := b.newAPIClient(state)
	if err != nil {
		return nil, err
	}
	steps := []multistep.Step{
		&StepCreateSSHKey{
			Debug:        b.config.PackerDebug,
			DebugKeyPath: fmt.Sprintf("ionos_%s", b.config.SnapshotName),
		},
		newStepCreateServer(client),
		&communicator.StepConnect{
			Config:    &b.config.Comm,
			Host:      communicator.CommHost(b.config.Comm.Host(), "server_ip"),
			SSHConfig: b.config.Comm.SSHConfigFunc(),
		},
		&commonsteps.StepProvision{},
		&commonsteps.StepCleanupTempKeys{
			Comm: &b.config.Comm,
		},
		newStepTakeSnapshot(client),
	}

	config := state.Get("config").(*Config)

	b.runner = commonsteps.NewRunner(steps, b.config.PackerConfig, ui)
	b.runner.Run(ctx, state)

	if rawErr, ok := state.GetOk("error"); ok {
		return nil, rawErr.(error)
	}

	artifact := &Artifact{
		snapshotData: config.SnapshotName,
		StateData:    map[string]interface{}{"generated_data": state.Get("generated_data")},
	}
	return artifact, nil
}

func (b *Builder) newAPIClient(state multistep.StateBag) (*ionoscloud.APIClient, error) {
	c := state.Get("config").(*Config)
	cfg := ionoscloud.NewConfiguration(c.IonosUsername, c.IonosPassword, "", "")
	cfg.SetDepth(5)

	// new apiclient for ionoscloud
	return ionoscloud.NewAPIClient(cfg), nil
}
