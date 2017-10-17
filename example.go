// This package demonstrates how to manage Azure virtual machines using Go.
package main

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/go-autorest/autorest/utils"
)

const (
	vhdURItemplate = "https://%s.blob.core.windows.net/golangcontainer/%s.vhd"
	linuxVMname    = "linuxVM"
	windowsVMname  = "windowsVM"
)

// This example requires that the following environment vars are set:
//
// AZURE_TENANT_ID: contains your Azure Active Directory tenant ID or domain
// AZURE_CLIENT_ID: contains your Azure Active Directory Application Client ID
// AZURE_CLIENT_SECRET: contains your Azure Active Directory Application Secret
// AZURE_SUBSCRIPTION_ID: contains your Azure Subscription ID
//

var (
	groupName   = "your-azure-sample-group"
	accountName = "golangrocksonazure"
	location    = "westus"
	vNetName    = "vNet"
	subnetName  = "subnet"

	groupClient      resources.GroupsClient
	accountClient    storage.AccountsClient
	vNetClient       network.VirtualNetworksClient
	subnetClient     network.SubnetsClient
	addressClient    network.PublicIPAddressesClient
	interfacesClient network.InterfacesClient
	vmClient         compute.VirtualMachinesClient
)

func init() {
	subscriptionID := getEnvVarOrExit("AZURE_SUBSCRIPTION_ID")

	authorizer, err := utils.GetAuthorizer(azure.PublicCloud)
	onErrorFail(err, "utils.GetAuthorizer failed")

	createClients(subscriptionID, authorizer)
}

func main() {

	subnet := createNeededResources()
	defer groupClient.Delete(groupName, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go createVM(linuxVMname, "Canonical", "UbuntuServer", "16.04.0-LTS", subnet, &wg)
	go createVM(windowsVMname, "MicrosoftWindowsServer", "WindowsServer", "2016-Datacenter", subnet, &wg)
	wg.Wait()

	fmt.Println("Your Linux VM and Windows VM have been created successfully")

	wg.Add(2)
	go vmOperations(linuxVMname, &wg)
	go vmOperations(windowsVMname, &wg)
	wg.Wait()

	listVMs()

	fmt.Print("Press enter to delete the VMs and other resources created in this sample...")
	var input string
	fmt.Scanln(&input)

	wg.Add(2)
	go deleteVM(linuxVMname, &wg)
	go deleteVM(windowsVMname, &wg)
	wg.Wait()

	fmt.Println("Delete resource group...")
	_, errChan := groupClient.Delete(groupName, nil)
	onErrorFail(<-errChan, "Delete failed")
}

// createNeededResources creates all common resources needed before creating VMs.
func createNeededResources() *network.Subnet {
	fmt.Println("Create needed resources")
	fmt.Printf("\tCreate resource group '%s'...\n", groupName)
	resourceGroupParameters := resources.Group{
		Location: &location,
	}

	_, err := groupClient.CreateOrUpdate(groupName, resourceGroupParameters)
	onErrorFail(err, fmt.Sprintf("groupClient.CreateOrUpdate failed for resource group '%s'", groupName))
	fmt.Printf("\tCreated resource group '%s' successfully\n", groupName)

	fmt.Printf("\tCreate storage account '%s'...\n", accountName)
	accountParameters := storage.AccountCreateParameters{
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
		},
		Location: &location,
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
	}

	_, errChan := accountClient.Create(groupName, accountName, accountParameters, nil)
	onErrorFail(<-errChan, fmt.Sprintf("accountClient.Create failed for storage account '%s'", accountName))
	fmt.Printf("\tCreated storage account '%s' successfully\n", accountName)

	vNetParameters := network.VirtualNetwork{
		Location: &location,
		VirtualNetworkPropertiesFormat: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{
				AddressPrefixes: &[]string{"10.0.0.0/16"},
			},
		},
	}

	fmt.Printf("\tCreate virtual network '%s'...\n", vNetName)
	_, errChan = vNetClient.CreateOrUpdate(groupName, vNetName, vNetParameters, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vNetClient.CreateOrUpdate failed for '%s'", vNetName))
	fmt.Printf("\tCreated virtual network '%s' successfully\n", vNetName)

	subnet := network.Subnet{
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("10.0.0.0/24"),
		},
	}

	fmt.Printf("\tCreate subnet '%s'...\n", subnetName)
	_, errChan = subnetClient.CreateOrUpdate(groupName, vNetName, subnetName, subnet, nil)
	onErrorFail(<-errChan, fmt.Sprintf("subnetClient.CreateOrUpdate failed for '%s'", subnetName))
	fmt.Printf("\tCreated subnet '%s'\n", subnetName)

	fmt.Printf("\tGet subnet info for sbunet '%s'...\n", subnetName)
	subnetInfo, err := subnetClient.Get(groupName, vNetName, subnetName, "")
	onErrorFail(err, fmt.Sprintf("subnetClient.Get failed for subnet '%s'", subnetName))

	return &subnetInfo
}

// createVM creates a VM in the provided subnet.
func createVM(vmName, publisher, offer, sku string, subnetInfo *network.Subnet, wg *sync.WaitGroup) {
	defer wg.Done()

	publicIPaddress, nicParameters := createPIPandNIC(vmName, subnetInfo)

	fmt.Printf("Create '%s' VM...\n", vmName)
	vm := setVMparameters(vmName, publisher, offer, sku, *nicParameters.ID)
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, vm, nil)
	onErrorFail(<-errChan, "createVM failed")

	fmt.Printf("Now you can connect to '%s' VM via 'ssh %s@%s' with password '%s'\n",
		vmName,
		*vm.OsProfile.AdminUsername,
		*publicIPaddress.DNSSettings.Fqdn,
		*vm.OsProfile.AdminPassword)
}

// createPIPandNIC creates a public IP address and a network interface in an existing subnet.
// It returns a network interface ready to be used to create a virtual machine.
func createPIPandNIC(machine string, subnetInfo *network.Subnet) (*network.PublicIPAddress, *network.Interface) {
	fmt.Printf("Create PIP and NIC for '%s' VM...\n", machine)
	IPname := fmt.Sprintf("pip-%s", machine)
	fmt.Printf("\tCreate public IP address '%s'...\n", IPname)
	pipParameters := network.PublicIPAddress{
		Location: &location,
		PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
			DNSSettings: &network.PublicIPAddressDNSSettings{
				DomainNameLabel: to.StringPtr(fmt.Sprintf("azuresample-%s", strings.ToLower(machine[:5]))),
			},
		},
	}

	_, errChan := addressClient.CreateOrUpdate(groupName, IPname, pipParameters, nil)
	onErrorFail(<-errChan, fmt.Sprintf("addressClient.CreateOrUpdate '%s' failed", IPname))
	fmt.Printf("\tCreated public IP address %s\n", IPname)

	fmt.Printf("\tGet public IP address info for '%s'...\n", IPname)
	publicIPaddress, err := addressClient.Get(groupName, IPname, "")
	onErrorFail(err, fmt.Sprintf("addressClient.Get for IP '%s' failed", IPname))

	nicName := fmt.Sprintf("nic-%s", machine)
	fmt.Printf("\tCreate NIC '%s'...\n", nicName)

	nicParameters := network.Interface{
		Location: &location,
		InterfacePropertiesFormat: &network.InterfacePropertiesFormat{
			IPConfigurations: &[]network.InterfaceIPConfiguration{
				{
					Name: to.StringPtr(fmt.Sprintf("IPconfig-%s", machine)),
					InterfaceIPConfigurationPropertiesFormat: &network.InterfaceIPConfigurationPropertiesFormat{
						PublicIPAddress:           &publicIPaddress,
						PrivateIPAllocationMethod: network.Dynamic,
						Subnet: subnetInfo,
					},
				},
			},
		},
	}

	_, errChan = interfacesClient.CreateOrUpdate(groupName, nicName, nicParameters, nil)
	onErrorFail(<-errChan, fmt.Sprintf("interfacesClient.CreateOrUpdate for NIC '%s' failed", nicName))
	fmt.Printf("\tCreated NIC '%s' successfully\n", nicName)

	fmt.Printf("\tGet NIC info for %s...\n", nicName)
	nicParameters, err = interfacesClient.Get(groupName, nicName, "")
	onErrorFail(err, fmt.Sprintf("interfaces.Get for NIC '%s' failed", nicName))

	return &publicIPaddress, &nicParameters
}

// setVMparameters builds the VirtualMachine argument for creating or updating a VM.
func setVMparameters(vmName, publisher, offer, sku, nicID string) compute.VirtualMachine {
	return compute.VirtualMachine{
		Location: &location,
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.VirtualMachineSizeTypesStandardDS1V2,
			},
			StorageProfile: &compute.StorageProfile{
				ImageReference: &compute.ImageReference{
					Publisher: &publisher,
					Offer:     &offer,
					Sku:       &sku,
					Version:   to.StringPtr("latest"),
				},
				OsDisk: &compute.OSDisk{
					Name: to.StringPtr("osDisk"),
					Vhd: &compute.VirtualHardDisk{
						URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, vmName)),
					},
					CreateOption: compute.DiskCreateOptionTypesFromImage,
				},
			},
			OsProfile: &compute.OSProfile{
				ComputerName:  &vmName,
				AdminUsername: to.StringPtr("notadmin"),
				AdminPassword: to.StringPtr("Pa$$w0rd1975"),
			},
			NetworkProfile: &compute.NetworkProfile{
				NetworkInterfaces: &[]compute.NetworkInterfaceReference{
					{
						ID: &nicID,
						NetworkInterfaceReferenceProperties: &compute.NetworkInterfaceReferenceProperties{
							Primary: to.BoolPtr(true),
						},
					},
				},
			},
		},
	}
}

// vmOperations performs simple VM operations.
func vmOperations(vmName string, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("Performing various operations on '%s' VM\n", vmName)
	vm := getVM(vmName)
	updateVM(vmName, vm)
	attachDataDisk(vmName, vm)
	detachDataDisks(vmName, vm)
	updateOSdiskSize(vmName, vm)
	startVM(vmName)
	restartVM(vmName)
	stopVM(vmName)
}

func getVM(vmName string) *compute.VirtualMachine {
	fmt.Printf("Get VM '%s' by name\n", vmName)
	vm, err := vmClient.Get(groupName, vmName, compute.InstanceView)
	onErrorFail(err, fmt.Sprintf("Get failed for '%s'", vmName))
	printVM(vm)
	return &vm
}

func updateVM(vmName string, vm *compute.VirtualMachine) {
	fmt.Printf("Tag VM '%s' (via CreateOrUpdate operation)\n", vmName)
	vm.Tags = &(map[string]*string{
		"who rocks": to.StringPtr("golang"),
		"where":     to.StringPtr("on azure"),
	})
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, fmt.Sprintf("CreateOrUpdate failed for VM '%s'", vmName))
}

func attachDataDisk(vmName string, vm *compute.VirtualMachine) {
	fmt.Printf("Attach data disk to VM '%s' (via CreateOrUpdate operation)\n", vmName)
	vm.StorageProfile.DataDisks = &[]compute.DataDisk{
		{
			Lun:  to.Int32Ptr(0),
			Name: to.StringPtr("dataDisk"),
			Vhd: &compute.VirtualHardDisk{
				URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, fmt.Sprintf("dataDisks-%v", vmName))),
			},
			CreateOption: compute.DiskCreateOptionTypesEmpty,
			DiskSizeGB:   to.Int32Ptr(1),
		},
	}
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.CreateOrUpdate failed for '%s'", vmName))
}

func detachDataDisks(vmName string, vm *compute.VirtualMachine) {
	fmt.Printf("Detach data disks from VM '%s' (via CreateOrUpdate operation)\n", vmName)
	vm.StorageProfile.DataDisks = &[]compute.DataDisk{}
	_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.CreateOrUpdate failed for '%s'", vmName))
}

func updateOSdiskSize(vmName string, vm *compute.VirtualMachine) {
	fmt.Printf("Update OS disk size for VM '%s' (via Deallocate and CreateOrUpdate operations)\n", vmName)
	if vm.StorageProfile.OsDisk.DiskSizeGB == nil {
		vm.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
	}
	_, errChan := vmClient.Deallocate(groupName, vmName, nil)
	onErrorFail(<-errChan, fmt.Sprintf("Deallocate failed for '%s'", vmName))
	if *vm.StorageProfile.OsDisk.DiskSizeGB <= 0 {
		*vm.StorageProfile.OsDisk.DiskSizeGB = 256
	}
	*vm.StorageProfile.OsDisk.DiskSizeGB += 10
	_, errChan = vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.CreateOrUpdate failed for '%s'", vmName))
}

func startVM(vmName string) {
	fmt.Println("Start VM...")
	_, errChan := vmClient.Start(groupName, vmName, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.Start failed for '%s'", vmName))
}

func restartVM(vmName string) {
	fmt.Println("Restart VM...")
	_, errChan := vmClient.Restart(groupName, vmName, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.Restart failed for '%s'", vmName))
}

func stopVM(vmName string) {
	fmt.Println("Stop VM...")
	_, errChan := vmClient.PowerOff(groupName, vmName, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.PowerOff failed for '%s'", vmName))
}

func listVMs() {
	fmt.Println("List VMs in subscription...")
	list, err := vmClient.ListAll()
	onErrorFail(err, "ListAll failed")
	if list.Value != nil && len(*list.Value) > 0 {
		fmt.Println("VMs in subscription")
		for _, vm := range *list.Value {
			printVM(vm)
		}
	} else {
		fmt.Println("There are no VMs in this subscription")
	}
}

func deleteVM(vmName string, wg *sync.WaitGroup) {
	defer wg.Done()

	fmt.Printf("Delete '%s' virtual machine...\n", vmName)
	_, errChan := vmClient.Delete(groupName, vmName, nil)
	onErrorFail(<-errChan, fmt.Sprintf("vmClient.Delete failed for '%s'", vmName))
}

// printVM prints basic info about a Virtual Machine.
func printVM(vm compute.VirtualMachine) {
	tags := "\n"
	if vm.Tags == nil {
		tags += "\t\tNo tags yet\n"
	} else {
		for k, v := range *vm.Tags {
			tags += fmt.Sprintf("\t\t%s = %s\n", k, *v)
		}
	}
	fmt.Printf("Virtual machine '%s'\n", *vm.Name)
	elements := map[string]interface{}{
		"ID":       *vm.ID,
		"Type":     *vm.Type,
		"Location": *vm.Location,
		"Tags":     tags}
	for k, v := range elements {
		fmt.Printf("\t%s: %s\n", k, v)
	}
}

// getEnvVarOrExit returns the value of specified environment variable or terminates if it's not defined.
func getEnvVarOrExit(varName string) string {
	value := os.Getenv(varName)
	if value == "" {
		fmt.Printf("Missing environment variable '%s'\n", varName)
		os.Exit(1)
	}

	return value
}

// onErrorFail prints a failure message and exits the program if err is not nil.
func onErrorFail(err error, message string) {
	if err != nil {
		fmt.Printf("%s: %s\n", message, err)
		os.Exit(1)
	}
}

func createClients(subscriptionID string, authorizer *autorest.BearerAuthorizer) {
	groupClient = resources.NewGroupsClient(subscriptionID)
	groupClient.Authorizer = authorizer

	accountClient = storage.NewAccountsClient(subscriptionID)
	accountClient.Authorizer = authorizer

	vNetClient = network.NewVirtualNetworksClient(subscriptionID)
	vNetClient.Authorizer = authorizer

	subnetClient = network.NewSubnetsClient(subscriptionID)
	subnetClient.Authorizer = authorizer

	addressClient = network.NewPublicIPAddressesClient(subscriptionID)
	addressClient.Authorizer = authorizer

	interfacesClient = network.NewInterfacesClient(subscriptionID)
	interfacesClient.Authorizer = authorizer

	vmClient = compute.NewVirtualMachinesClient(subscriptionID)
	vmClient.Authorizer = authorizer
}
