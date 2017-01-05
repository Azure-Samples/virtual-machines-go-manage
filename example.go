// This package demonstrates how to manage Azure virtual machines using Go.
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/autorest/to"
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
	tenantID := getEnvVarOrExit("AZURE_TENANT_ID")

	oauthConfig, err := azure.PublicCloud.OAuthConfigForTenant(tenantID)
	onErrorFail(err, "OAuthConfigForTenant failed")

	clientID := getEnvVarOrExit("AZURE_CLIENT_ID")
	clientSecret := getEnvVarOrExit("AZURE_CLIENT_SECRET")
	spToken, err := azure.NewServicePrincipalToken(*oauthConfig, clientID, clientSecret, azure.PublicCloud.ResourceManagerEndpoint)
	onErrorFail(err, "NewServicePrincipalToken failed")

	createClients(subscriptionID, spToken)
}

func main() {
	subnetInfo, err := createNeededResources()
	onErrorFail(err, "createNeededResources failed")
	defer groupClient.Delete(groupName, nil)

	err = createVM(linuxVMname, "Canonical", "UbuntuServer", "16.04.0-LTS", subnetInfo)
	onErrorFail(err, "createVM failed")

	vmOperations(linuxVMname)

	err = createVM(windowsVMname, "MicrosoftWindowsServerEssentials", "WindowsServerEssentials", "WindowsServerEssentials", subnetInfo)
	onErrorFail(err, "createVM failed")

	vmOperations(windowsVMname)

	listVMs()

	fmt.Println("Your Linux VM and Windows VM have been created")
	fmt.Print("Press enter to delete the VMs and other resources created in this sample...")

	var input string
	fmt.Scanln(&input)

	deleteVM(linuxVMname)
	deleteVM(windowsVMname)

	fmt.Println("Delete resource group...")
	_, err = groupClient.Delete(groupName, nil)
	onErrorFail(err, "Delete failed")
}

// createNeededResources creates all common resources needed before creating VMs.
func createNeededResources() (*network.Subnet, error) {
	fmt.Println("Create needed resources")
	fmt.Println("\tCreate resource group...")
	resourceGroupParameters := resources.ResourceGroup{
		Location: &location,
	}
	if _, err := groupClient.CreateOrUpdate(groupName, resourceGroupParameters); err != nil {
		return nil, err
	}

	fmt.Println("\tCreate storage account...")
	accountParameters := storage.AccountCreateParameters{
		Sku: &storage.Sku{
			Name: storage.StandardLRS,
		},
		Location: &location,
		AccountPropertiesCreateParameters: &storage.AccountPropertiesCreateParameters{},
	}
	if _, err := accountClient.Create(groupName, accountName, accountParameters, nil); err != nil {
		return nil, err
	}

	fmt.Println("\tCreate virtual network...")
	vNetName := "vNet"
	vNetParameters := network.VirtualNetwork{
		Location: &location,
		VirtualNetworkPropertiesFormat: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{
				AddressPrefixes: &[]string{"10.0.0.0/16"},
			},
		},
	}
	if _, err := vNetClient.CreateOrUpdate(groupName, vNetName, vNetParameters, nil); err != nil {
		return nil, err
	}

	fmt.Println("\tCreate subnet...")
	subnetName := "subnet"
	subnet := network.Subnet{
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("10.0.0.0/24"),
		},
	}
	if _, err := subnetClient.CreateOrUpdate(groupName, vNetName, subnetName, subnet, nil); err != nil {
		return nil, err
	}

	fmt.Println("\tGet subnet info...")
	subnetInfo, err := subnetClient.Get(groupName, vNetName, subnetName, "")
	if err != nil {
		return nil, err
	}

	return &subnetInfo, err
}

// createVM creates a VM in the provided subnet.
func createVM(vmName, publisher, offer, sku string, subnetInfo *network.Subnet) error {
	publicIPaddress, nicParameters, err := createPIPandNIC(vmName, subnetInfo)
	if err != nil {
		return err
	}

	fmt.Printf("Create '%s' VM...\n", vmName)
	vm := setVMparameters(vmName, publisher, offer, sku, *nicParameters.ID)
	if _, err := vmClient.CreateOrUpdate(groupName, vmName, vm, nil); err != nil {
		return err
	}

	fmt.Printf("Now you can connect to '%s' VM via 'ssh %s@%s' with password '%s'\n",
		vmName,
		*vm.OsProfile.AdminUsername,
		*publicIPaddress.DNSSettings.Fqdn,
		*vm.OsProfile.AdminPassword)

	return nil
}

// createPIPandNIC creates a public IP address and a network interface in an existing subnet.
// It returns a network interface ready to be used to create a virtual machine.
func createPIPandNIC(machine string, subnetInfo *network.Subnet) (*network.PublicIPAddress, *network.Interface, error) {
	fmt.Printf("Create PIP and NIC for %s VM...\n", machine)
	fmt.Println("\tCreate public IP address...")
	IPname := fmt.Sprintf("pip-%s", machine)
	pipParameters := network.PublicIPAddress{
		Location: &location,
		PublicIPAddressPropertiesFormat: &network.PublicIPAddressPropertiesFormat{
			DNSSettings: &network.PublicIPAddressDNSSettings{
				DomainNameLabel: to.StringPtr(fmt.Sprintf("azuresample-%s", strings.ToLower(machine[:5]))),
			},
		},
	}
	if _, err := addressClient.CreateOrUpdate(groupName, IPname, pipParameters, nil); err != nil {
		return nil, nil, err
	}

	fmt.Println("\tGet public IP address info...")
	publicIPaddress, err := addressClient.Get(groupName, IPname, "")
	if err != nil {
		return nil, nil, err
	}

	fmt.Println("\tCreate NIC...")
	nicName := fmt.Sprintf("nic-%s", machine)
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
	if _, err := interfacesClient.CreateOrUpdate(groupName, nicName, nicParameters, nil); err != nil {
		return &publicIPaddress, nil, err
	}

	fmt.Println("\tGet NIC info...")
	nicParameters, err = interfacesClient.Get(groupName, nicName, "")
	if err != nil {
		return &publicIPaddress, nil, err
	}

	return &publicIPaddress, &nicParameters, nil
}

// setVMparameters builds the VirtualMachine argument for creating or updating a VM.
func setVMparameters(vmName, publisher, offer, sku, nicID string) compute.VirtualMachine {
	return compute.VirtualMachine{
		Location: &location,
		VirtualMachineProperties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.StandardDS1,
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
					CreateOption: compute.FromImage,
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
func vmOperations(vmName string) {
	fmt.Printf("Performing various operations on '%s' VM\n", vmName)
	vm := getVM(vmName)

	// weird SDK usage caused by this issue
	// https://github.com/Azure/autorest/issues/1559
	vm.ProvisioningState = nil
	vm.InstanceView = nil
	vm.VMID = nil

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
	onErrorFail(err, "Get failed")
	printVM(vm)
	return &vm
}

func updateVM(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Tag VM (via CreateOrUpdate operation)")
	vm.Tags = &(map[string]*string{
		"who rocks": to.StringPtr("golang"),
		"where":     to.StringPtr("on azure"),
	})
	_, err := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(err, "CreateOrUpdate failed")
}

func attachDataDisk(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Attach data disk (via CreateOrUpdate operation)")
	vm.StorageProfile.DataDisks = &[]compute.DataDisk{
		{
			Lun:  to.Int32Ptr(0),
			Name: to.StringPtr("dataDisk"),
			Vhd: &compute.VirtualHardDisk{
				URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, fmt.Sprintf("dataDisks-%v", vmName))),
			},
			CreateOption: compute.Empty,
			DiskSizeGB:   to.Int32Ptr(1),
		},
	}
	_, err := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(err, "CreateOrUpdate failed")
}

func detachDataDisks(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Detach data disks (via CreateOrUpdate operation)")
	vm.StorageProfile.DataDisks = &[]compute.DataDisk{}
	_, err := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(err, "CreateOrUpdate failed")
}

func updateOSdiskSize(vmName string, vm *compute.VirtualMachine) {
	fmt.Println("Update OS disk size (via Deallocate and CreateOrUpdate operations)")
	if vm.StorageProfile.OsDisk.DiskSizeGB == nil {
		vm.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
	}
	_, err := vmClient.Deallocate(groupName, vmName, nil)
	onErrorFail(err, "Deallocate failed")
	if *vm.StorageProfile.OsDisk.DiskSizeGB <= 0 {
		*vm.StorageProfile.OsDisk.DiskSizeGB = 256
	}
	*vm.StorageProfile.OsDisk.DiskSizeGB += 10
	_, err = vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
	onErrorFail(err, "CreateOrUpdate failed")
}

func startVM(vmName string) {
	fmt.Println("Start VM...")
	_, err := vmClient.Start(groupName, vmName, nil)
	onErrorFail(err, "Start failed")
}

func restartVM(vmName string) {
	fmt.Println("Restart VM...")
	_, err := vmClient.Restart(groupName, vmName, nil)
	onErrorFail(err, "Restart failed")
}

func stopVM(vmName string) {
	fmt.Println("Stop VM...")
	_, err := vmClient.PowerOff(groupName, vmName, nil)
	onErrorFail(err, "Stop failed")
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

func deleteVM(vmName string) {
	fmt.Printf("Delete '%s' virtual machine...\n", vmName)
	_, err := vmClient.Delete(groupName, vmName, nil)
	onErrorFail(err, "Delete failed")
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
		fmt.Printf("Missing environment variable %s\n", varName)
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

func createClients(subscriptionID string, spToken *azure.ServicePrincipalToken) {
	groupClient = resources.NewGroupsClient(subscriptionID)
	groupClient.Authorizer = spToken

	accountClient = storage.NewAccountsClient(subscriptionID)
	accountClient.Authorizer = spToken

	vNetClient = network.NewVirtualNetworksClient(subscriptionID)
	vNetClient.Authorizer = spToken

	subnetClient = network.NewSubnetsClient(subscriptionID)
	subnetClient.Authorizer = spToken

	addressClient = network.NewPublicIPAddressesClient(subscriptionID)
	addressClient.Authorizer = spToken

	interfacesClient = network.NewInterfacesClient(subscriptionID)
	interfacesClient.Authorizer = spToken

	vmClient = compute.NewVirtualMachinesClient(subscriptionID)
	vmClient.Authorizer = spToken
}
