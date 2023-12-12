// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package ionoscloud

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/hashicorp/packer-plugin-sdk/multistep"
	packersdk "github.com/hashicorp/packer-plugin-sdk/packer"
	ionoscloud "github.com/ionos-cloud/sdk-go/v6"
)

type stepCreateServer struct {
	client *ionoscloud.APIClient
}

func newStepCreateServer(client *ionoscloud.APIClient) *stepCreateServer {
	return &stepCreateServer{
		client: client,
	}
}

func (s *stepCreateServer) Run(ctx context.Context, state multistep.StateBag) multistep.StepAction {
	ui := state.Get("ui").(packersdk.Ui)
	c := state.Get("config").(*Config)

	ui.Say("Creating Virtual Data Center...")
	img, err := s.getImageId(c.Image, c)
	if err != nil {
		ui.Error(fmt.Sprintf("Error occurred while getting image %s", err.Error()))
		return multistep.ActionHalt
	}

	props := &ionoscloud.VolumeProperties{
		Type:  ionoscloud.PtrString(c.DiskType),
		Size:  ionoscloud.PtrFloat32(c.DiskSize),
		Name:  ionoscloud.PtrString(c.SnapshotName),
		Image: ionoscloud.PtrString(img),
	}
	nic := ionoscloud.Nic{
		Properties: &ionoscloud.NicProperties{
			Name: ionoscloud.PtrString(c.SnapshotName),
			Dhcp: ionoscloud.PtrBool(true),
		},
	}
	if c.Comm.SSHPassword != "" {
		props.ImagePassword = ionoscloud.PtrString(c.Comm.SSHPassword)
	}
	if c.Comm.SSHPublicKey != nil {
		props.SshKeys = &[]string{string(c.Comm.SSHPublicKey)}
	}
	serverReq := ionoscloud.Server{
		Properties: &ionoscloud.ServerProperties{
			Name:  ionoscloud.PtrString(c.SnapshotName),
			Ram:   ionoscloud.PtrInt32(c.Ram),
			Cores: ionoscloud.PtrInt32(c.Cores),
		},
		Entities: &ionoscloud.ServerEntities{
			Volumes: &ionoscloud.AttachedVolumes{
				Items: &[]ionoscloud.Volume{
					{
						Properties: props,
					},
				},
			},
			Nics: &ionoscloud.Nics{
				Items: &[]ionoscloud.Nic{
					nic,
				},
			},
		},
	}

	// create datacenter
	dc, err := s.createDcAndWaitUntilDone(ctx, c.SnapshotName, c.Region)
	if err != nil {
		ui.Error(fmt.Sprintf("Error occurred while creating a datacenter %s", err.Error()))
		return multistep.ActionHalt
	}
	dcId := *dc.Id
	state.Put("datacenter_id", dcId)

	lanPost := ionoscloud.LanPost{
		Properties: &ionoscloud.LanPropertiesPost{
			Public: ionoscloud.PtrBool(true),
			Name:   ionoscloud.PtrString(c.SnapshotName),
		},
	}

	ui.Say("Creating LAN...")
	// create lan
	lan, err := s.createLanAndWaitUntilDone(ctx, dcId, lanPost)
	if err != nil {
		ui.Error(fmt.Sprintf("Error occurred while creating a server %s", err.Error()))
		return multistep.ActionHalt
	}

	// string to int
	lanId, err := strconv.Atoi(*lan.Id)
	if err != nil {
		ui.Error(fmt.Sprintf("Error occurred while creating a server %s", err.Error()))
		return multistep.ActionHalt
	}
	nic.Properties.Lan = ionoscloud.PtrInt32(int32(lanId))

	ui.Say("Creating Server...")
	// create server
	server, err := s.createServerAndWaitUntilDone(ctx, dcId, serverReq)
	if err != nil {
		ui.Error(fmt.Sprintf("Error occurred while creating a server %s", err.Error()))
		return multistep.ActionHalt
	}

	volumes := *server.Entities.Volumes.Items
	state.Put("volume_id", *volumes[0].Id)

	server, err = s.findServerById(ctx, dcId, *server.Id)
	if err != nil {
		ui.Error(fmt.Sprintf("Error occurred while finding the server %s", err.Error()))
		return multistep.ActionHalt
	}

	// instance_id is the generic term used so that users can have access to the
	// instance id inside of the provisioners, used in step_provision.
	state.Put("instance_id", *server.Id)

	nics := *server.Entities.Nics.Items
	ips := *nics[0].Properties.Ips
	state.Put("server_ip", ips[0])
	ui.Say("Server Created...")

	return multistep.ActionContinue
}

func (s *stepCreateServer) Cleanup(state multistep.StateBag) {
	ui := state.Get("ui").(packersdk.Ui)

	ui.Say("Removing Virtual Data Center...")

	if dcId, ok := state.GetOk("datacenter_id"); ok {
		deleted, err := s.deleteDatacenter(s.client, dcId.(string))
		if err != nil {
			ui.Error(fmt.Sprintf(
				"Error deleting Virtual Data Center. Please destroy it manually: %s", err))
		}
		if deleted {
			ui.Say("Virtual Data Center deleted...")
		}
	}
}

func processRequestDatacenterDelete(apiClient *ionoscloud.APIClient, resourceID string) (*ionoscloud.APIResponse, error) {
	apiRequest := apiClient.DataCentersApi.DatacentersFindById(context.Background(), resourceID)
	_, apiResp, err := apiRequest.Execute()
	if err != nil {
		return apiResp, fmt.Errorf("error occurred when executing the api get resource operation: %w", err)
	}

	return apiResp, nil
}

func (s *stepCreateServer) deleteDatacenter(apiClient *ionoscloud.APIClient, datacenterID string) (bool, error) {
	_, err := apiClient.DataCentersApi.DatacentersDelete(context.Background(), datacenterID).Execute()
	if err != nil {
		return false, fmt.Errorf("error occurred when executing the api get resource operation: %w", err)
	}
	return apiClient.WaitForDeletion(context.Background(), processRequestDatacenterDelete, datacenterID)
}

func (s *stepCreateServer) getImageId(imageName string, c *Config) (string, error) {
	images, resp, err := s.client.ImagesApi.ImagesGet(context.Background()).Execute()
	if err != nil {
		return "", err
	}
	if resp.StatusCode > 299 {
		return "", errors.New("error occurred while getting images")
	}

	for i := 0; i < len(*images.Items); i++ {
		imgName := ""
		items := *images.Items
		if *items[i].Properties.Name != "" {
			imgName = *items[i].Properties.Name
		}
		diskType := c.DiskType
		if c.DiskType == "SSD" {
			diskType = "HDD"
		}
		if imgName != "" && strings.Contains(strings.ToLower(imgName), strings.ToLower(imageName)) && *items[i].Properties.ImageType == diskType && *items[i].Properties.Location == c.Region && *items[i].Properties.Public {
			return *items[i].Id, nil
		}
	}
	return "", nil
}

// createDcAndWaitUntilDone - creates datacenter and waits until provisioning is successful
// return - datacenter object created, or error
func (s *stepCreateServer) createDcAndWaitUntilDone(ctx context.Context, name, loc string) (*ionoscloud.Datacenter, error) {

	//datacenterName := "testDatacenter"
	description := "this is the packer datacenter"
	//loc := "de/txl" // other location values: de/fra", "us/las", "us/ewr", "de/txl", gb/lhr", "es/vit"
	dc, apiResponse, err := s.createDatacenter(ctx, name, description, loc)
	if err != nil {
		return nil, fmt.Errorf(
			"error creating data center (%w)", err)
	}
	// gets the Location Header value, where Request ID is stored, to interrogate the request status
	requestPath := getRequestPath(apiResponse)
	if requestPath == "" {
		return nil, fmt.Errorf("error getting location from header for datacenter")
	}

	// Waits for the datacenter creation to finish. Polls until it receives an answer that
	// provisioning is successful
	err = s.waitForRequestToBeDone(ctx, requestPath)
	if err != nil {
		return nil, fmt.Errorf("error while waiting for datacenter creation to finish (%w)", err)
	}
	return &dc, nil
}

func (s *stepCreateServer) createDatacenter(ctx context.Context, name, description, location string) (ionoscloud.Datacenter, *ionoscloud.APIResponse, error) {
	// The required parameter for datacenter creation is: 'location'.
	// Creates the datacenter structure and populates it
	dc := ionoscloud.Datacenter{
		Properties: &ionoscloud.DatacenterProperties{
			Name:        &name,
			Description: &description,
			Location:    &location,
		},
	}
	// The computeClient has access to all the resources in the ionos compute ecosystem. First we get the DatacenterApi.
	// The datacenter is the basic building block in which to create your infrastructure.
	// Builder pattern is used, to allow for easier creation and cleaner code.
	// In this case, the order is DatacentersPost -> Datacenter (loads datacenter structure) -> Execute
	// The final step that actually sends the request is 'execute'.
	return s.client.DataCentersApi.DatacentersPost(ctx).Datacenter(dc).Execute()
}

// waitForRequestToBeDone - polls until the request is 'Done', or
// until the context timeout expires
func (s *stepCreateServer) waitForRequestToBeDone(ctx context.Context, path string) error {
	// Polls until context timeout expires
	_, err := s.client.WaitForRequest(ctx, path)
	if err != nil {
		return fmt.Errorf("error waiting for status for %s : (%w)", path, err)
	}
	log.Printf("resource created for path %s", path)
	return nil
}

// createServerAndWaitUntilDone - creates server and waits until provisioning is successful
// return - server object created, or error
func (s *stepCreateServer) createServerAndWaitUntilDone(ctx context.Context, dcId string, server ionoscloud.Server) (*ionoscloud.Server, error) {
	server, apiResponse, err := s.createServer(ctx, dcId, server)
	if err != nil {
		return nil, fmt.Errorf(
			"error creating server (%w)", err)
	}
	// The initial response from the cloud is a HTTP/2.0 202 Accepted - after this response, the IONOS Cloud API
	// starts to actually create the server
	// Gets path to interrogate server creation status
	requestPath := getRequestPath(apiResponse)
	if requestPath == "" {
		return nil, fmt.Errorf("error getting server path")
	}
	// Waits for the server creation to finish. It takes some time to create
	// a compute resource, so we poll until provisioning is successful
	err = s.waitForRequestToBeDone(ctx, requestPath)
	if err != nil {
		return nil, fmt.Errorf("error while waiting for server creation to finish (%w)", err)
	}
	return &server, nil
}

func (s *stepCreateServer) createServer(ctx context.Context, datacenterId string, server ionoscloud.Server) (ionoscloud.Server, *ionoscloud.APIResponse, error) {
	return s.client.ServersApi.DatacentersServersPost(ctx, datacenterId).Server(server).Execute()
}

// createLanAndWaitUntilDone - creates LAN and waits until provisioning is successful
// return - server object created, or error
func (s *stepCreateServer) createLanAndWaitUntilDone(ctx context.Context, dcId string, lanPost ionoscloud.LanPost) (*ionoscloud.LanPost, error) {
	lan, apiResponse, err := s.client.LANsApi.DatacentersLansPost(ctx, dcId).Lan(lanPost).Execute()
	if err != nil {
		return nil, fmt.Errorf(
			"error creating LAN (%w)", err)
	}
	// The initial response from the cloud is a HTTP/2.0 202 Accepted - after this response, the IONOS Cloud API
	// starts to actually create the server
	// Gets path to interrogate server creation status
	requestPath := getRequestPath(apiResponse)
	if requestPath == "" {
		return nil, fmt.Errorf("error getting LAN path")
	}
	// Waits for the server creation to finish. It takes some time to create
	// a compute resource, so we poll until provisioning is successful
	err = s.waitForRequestToBeDone(ctx, requestPath)
	if err != nil {
		return nil, fmt.Errorf("error while waiting for LAN creation to finish (%w)", err)
	}
	return &lan, nil
}

// findServerById - finds a server by id
func (s *stepCreateServer) findServerById(ctx context.Context, dcId, serverID string) (*ionoscloud.Server, error) {
	server, _, err := s.client.ServersApi.DatacentersServersFindById(ctx, dcId, serverID).Execute()
	if err != nil {
		return nil, fmt.Errorf(
			"error attaching NIC (%w)", err)
	}
	return &server, nil
}

// getRequestPath - returns location header value which is the path
// used to poll the request for readiness
func getRequestPath(resp *ionoscloud.APIResponse) string {
	if resp != nil {
		return resp.Header.Get("location")
	}
	return ""
}
