// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ionoscloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	ionoscloud "github.com/ionos-cloud/sdk-go/v6"
)

type stepTakeSnapshot struct {
	client *ionoscloud.APIClient
}

func newStepTakeSnapshot(client *ionoscloud.APIClient) *stepTakeSnapshot {
	return &stepTakeSnapshot{
		client: client,
	}
}

func (s *stepTakeSnapshot) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	c := state.Get("config").(*Config)

	ui.Say("Creating IONOS snapshot...")

	dcId := state.Get("datacenter_id").(string)
	volumeId := state.Get("volume_id").(string)
	serverId := state.Get("instance_id").(string)

	comm, _ := state.Get("communicator").(packersdk.Communicator)
	if comm == nil {
		ui.Error("no communicator found")
		return multistep.ActionHalt
	}

	/* sync fs changes from the provisioning step */
	os, err := s.getOs(ctx, dcId, serverId)
	if err != nil {
		ui.Error(fmt.Sprintf("an error occurred while getting the server os: %s", err.Error()))
		return multistep.ActionHalt
	}
	ui.Say(fmt.Sprintf("Server OS is %s", os))

	switch strings.ToLower(os) {
	case "linux":
		ui.Say("syncing file system changes")
		if err := s.syncFs(ctx, comm); err != nil {
			ui.Error(fmt.Sprintf("error syncing fs changes: %s", err.Error()))
			return multistep.ActionHalt
		}
	}

	ui.Say(fmt.Sprintf("Creating a snapshot for %s/volumes/%s", dcId, volumeId))
	snapshot, err := s.createSnapshot(ctx, dcId, volumeId)
	if err != nil {
		ui.Error(fmt.Sprintf("An error occurred while creating a snapshot: %s", err.Error()))
		return multistep.ActionHalt
	}

	state.Put("snapshotname", c.SnapshotName)

	ui.Say("Waiting until snapshot available snapshot")

	err = s.waitTillSnapshotAvailable(*snapshot.Id, ui)
	if err != nil {
		ui.Error(fmt.Sprintf("An error occurred while waiting for the snapshot to be created: %s", err.Error()))
		return multistep.ActionHalt
	}

	return multistep.ActionContinue
}

func (s *stepTakeSnapshot) Cleanup(_ multistep.StateBag) {
}

func processRequestSnapshot(apiClient *ionoscloud.APIClient, resourceID string) (ionoscloud.ResourceHandler, error) {
	apiRequest := apiClient.SnapshotsApi.SnapshotsFindById(context.Background(), resourceID)
	dc, _, err := apiRequest.Execute()
	if err != nil {
		return nil, err
	}
	return &dc, nil
}

func (s *stepTakeSnapshot) waitTillSnapshotAvailable(id string, ui packersdk.Ui) error {
	available, err := s.client.WaitForState(context.Background(), processRequestSnapshot, id)
	if err != nil {
		return err
	}
	if available {
		ui.Say("snapshot available")
		return nil
	} else {
		return errors.New("snapshot not available")
	}
}

func (s *stepTakeSnapshot) syncFs(ctx context.Context, comm packersdk.Communicator) error {
	cmd := &packersdk.RemoteCmd{
		Command: "sync",
	}
	if err := comm.Start(ctx, cmd); err != nil {
		return err
	}
	if cmd.Wait() != 0 {
		return fmt.Errorf("sync command exited with code %d", cmd.ExitStatus())
	}
	return nil
}

func (s *stepTakeSnapshot) getOs(ctx context.Context, dcId string, serverId string) (string, error) {
	server, resp, err := s.client.ServersApi.DatacentersServersFindById(ctx, dcId, serverId).Execute()
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", errors.New(resp.Message)
	}

	if server.Properties.BootVolume == nil {
		return "", errors.New("no boot volume found on server")
	}

	volumeId := *server.Properties.BootVolume.Id
	volume, resp, err := s.client.VolumesApi.DatacentersVolumesFindById(ctx, dcId, volumeId).Execute()
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", errors.New(resp.Message)
	}

	return *volume.Properties.LicenceType, nil
}

func (s *stepTakeSnapshot) createSnapshot(ctx context.Context, dcId string, volumeId string) (*ionoscloud.Snapshot, error) {
	snapshot, apiResponse, err := s.client.VolumesApi.DatacentersVolumesCreateSnapshotPost(ctx, dcId, volumeId).Execute()
	if err != nil {
		return nil, fmt.Errorf(
			"error creating snapshot (%w)", err)
	}

	// gets the Location Header value, where Request ID is stored, to interrogate the request status
	requestPath := getRequestPath(apiResponse)
	if requestPath == "" {
		return nil, fmt.Errorf("error getting location from header for datacenter")
	}

	// Waits for the snapshot creation to finish. Polls until it receives an answer that
	// provisioning is successful
	err = s.waitForRequestToBeDone(ctx, requestPath)
	if err != nil {
		return nil, fmt.Errorf("error while waiting for datacenter creation to finish (%w)", err)
	}

	return &snapshot, nil
}

// waitForRequestToBeDone - polls until the request is 'Done', or
// until the context timeout expires
func (s *stepTakeSnapshot) waitForRequestToBeDone(ctx context.Context, path string) error {
	// Polls until context timeout expires
	_, err := s.client.WaitForRequest(ctx, path)
	if err != nil {
		return fmt.Errorf("error waiting for status for %s : (%w)", path, err)
	}
	log.Printf("resource created for path %s", path)
	return nil
}
