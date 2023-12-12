// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package ionoscloud

import (
	"errors"
	"os"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/communicator"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/mitchellh/mapstructure"
)

type Config struct {
	common.PackerConfig `mapstructure:",squash"`
	Comm                communicator.Config `mapstructure:",squash"`

	IonosUsername string `mapstructure:"username"`
	IonosPassword string `mapstructure:"password"`
	IonosApiUrl   string `mapstructure:"url"`

	Region       string  `mapstructure:"location"`
	Image        string  `mapstructure:"image"`
	SnapshotName string  `mapstructure:"snapshot_name"`
	DiskSize     float32 `mapstructure:"disk_size"`
	DiskType     string  `mapstructure:"disk_type"`
	Cores        int32   `mapstructure:"cores"`
	Ram          int32   `mapstructure:"ram"`
	Retries      int     `mapstructure:"retries"`
	ctx          interpolate.Context
}

func (c *Config) Prepare(raws ...interface{}) ([]string, error) {

	var md mapstructure.Metadata
	err := config.Decode(c, &config.DecodeOpts{
		Metadata:           &md,
		Interpolate:        true,
		InterpolateContext: &c.ctx,
		InterpolateFilter: &interpolate.RenderFilter{
			Exclude: []string{
				"run_command",
			},
		},
	}, raws...)
	if err != nil {
		return nil, err
	}

	var errs *packersdk.MultiError

	if err := c.Comm.Prepare(&c.ctx); err != nil {
		errs = packersdk.MultiErrorAppend(
			errs, err...)
	}

	if c.Comm.SSHPassword == "" && c.Comm.SSHPrivateKeyFile == "" {
		errs = packersdk.MultiErrorAppend(
			errs, errors.New("either ssh private key path or ssh password must be set"))
	}

	if c.SnapshotName == "" {
		def, err := interpolate.Render("packer-{{timestamp}}", nil)
		if err != nil {
			panic(err)
		}

		// Default to packer-{{ unix timestamp (utc) }}
		c.SnapshotName = def
	}

	if c.IonosUsername == "" {
		c.IonosUsername = os.Getenv("IONOS_USERNAME")
	}

	if c.IonosPassword == "" {
		c.IonosPassword = os.Getenv("IONOS_PASSWORD")
	}

	if c.IonosApiUrl == "" {
		c.IonosApiUrl = "https://api.ionos.com"
	}

	if c.Cores == 0 {
		c.Cores = 4
	}

	if c.Ram == 0 {
		c.Ram = 2048
	}

	if c.DiskSize == 0 {
		c.DiskSize = 50
	}

	if c.Region == "" {
		c.Region = "us/las"
	}

	if c.DiskType == "" {
		c.DiskType = "HDD"
	}

	if es := c.Comm.Prepare(&c.ctx); len(es) > 0 {
		errs = packersdk.MultiErrorAppend(errs, es...)
	}

	if c.Image == "" {
		errs = packersdk.MultiErrorAppend(
			errs, errors.New("IONOS 'image' is required"))
	}

	if c.IonosUsername == "" {
		errs = packersdk.MultiErrorAppend(
			errs, errors.New("IONOS username is required"))
	}

	if c.IonosPassword == "" {
		errs = packersdk.MultiErrorAppend(
			errs, errors.New("IONOS password is required"))
	}

	if errs != nil && len(errs.Errors) > 0 {
		return nil, errs
	}
	packersdk.LogSecretFilter.Set(c.IonosUsername)

	return nil, nil
}
