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

var (
	resourceGroupName = "VMsampleResourceGroup"
	accountName       = "mystorageaccount"
	location          = "westus"
	vhdURItemplate    = "https://%s.blob.core.windows.net/golangcontainer/%s.vhd"

	groupClient      resources.GroupsClient
	accountClient    storage.AccountsClient
	vNetClient       network.VirtualNetworksClient
	subnetClient     network.SubnetsClient
	addressClient    network.PublicIPAddressesClient
	interfacesClient network.InterfacesClient
	vmClient         compute.VirtualMachinesClient
)

func main() {
	subnetInfo, err := setup()
	if err != nil {
		printError(err)
		return
	}

	linuxVMname, windowsVMname := "linuxVM", "windowsVM"

	if err := createVM(linuxVMname, "Canonical", "UbuntuServer", "16.04.0-LTS", subnetInfo); err != nil {
		printError(err)
		return
	}
	printError(vmOperations(linuxVMname))

	if err := createVM(windowsVMname, "MicrosoftWindowsServerEssentials", "WindowsServerEssentials", "WindowsServerEssentials", subnetInfo); err != nil {
		printError(err)
		return
	}
	printError(vmOperations(windowsVMname))

	fmt.Println("List VMs in subscription...")
	vmList, err := vmClient.ListAll()
	if err != nil {
		printError(err)
	} else {
		if vmList.Value != nil {
			fmt.Println("VMs in subscription")
			for _, vm := range *vmList.Value {
				printVM(vm)
			}
		} else {
			fmt.Println("No VMs in subscription")
		}
	}
	/*
		fmt.Printf("Delete '%s' virtual machine...\n", linuxVMname)
		_, err = vmClient.Delete(resourceGroupName, linuxVMname, nil)
		printError(err)

		fmt.Printf("Delete '%s' virtual machine...\n", windowsVMname)
		_, err = vmClient.Delete(resourceGroupName, windowsVMname, nil)
		printError(err)

		fmt.Println("Delete resource group...")
		_, err = groupClient.Delete(resourceGroupName, nil)
		printError(err)
	*/
}

// setup performs all needed operations before creating a VM, including getting credentials, setting up clients and creating resources.
func setup() (*network.Subnet, error) {
	credentials, err := getCredentials()
	if err != nil {
		return nil, err
	}
	token, err := getToken(credentials)
	if err != nil {
		return nil, err
	}

	groupClient = resources.NewGroupsClient(credentials["AZURE_SUBSCRIPTION_ID"])
	groupClient.Authorizer = token

	accountClient = storage.NewAccountsClient(credentials["AZURE_SUBSCRIPTION_ID"])
	accountClient.Authorizer = token

	vNetClient = network.NewVirtualNetworksClient(credentials["AZURE_SUBSCRIPTION_ID"])
	vNetClient.Authorizer = token

	subnetClient = network.NewSubnetsClient(credentials["AZURE_SUBSCRIPTION_ID"])
	subnetClient.Authorizer = token

	addressClient = network.NewPublicIPAddressesClient(credentials["AZURE_SUBSCRIPTION_ID"])
	addressClient.Authorizer = token

	interfacesClient = network.NewInterfacesClient(credentials["AZURE_SUBSCRIPTION_ID"])
	interfacesClient.Authorizer = token

	vmClient = compute.NewVirtualMachinesClient(credentials["AZURE_SUBSCRIPTION_ID"])
	vmClient.Authorizer = token

	subnetInfo, err := createNeededResources()
	if err != nil {
		return nil, err
	}

	return subnetInfo, nil
}

// getCredentials gets some credentials from your environment variables.
func getCredentials() (map[string]string, error) {
	credentials := map[string]string{
		"AZURE_CLIENT_ID":       os.Getenv("AZURE_CLIENT_ID"),
		"AZURE_CLIENT_SECRET":   os.Getenv("AZURE_CLIENT_SECRET"),
		"AZURE_SUBSCRIPTION_ID": os.Getenv("AZURE_SUBSCRIPTION_ID"),
		"AZURE_TENANT_ID":       os.Getenv("AZURE_TENANT_ID")}
	if err := checkEnvVar(&credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

// checkEnvVar checks if the environment variables are actually set.
func checkEnvVar(envVars *map[string]string) error {
	var missingVars []string
	for varName, value := range *envVars {
		if value == "" {
			missingVars = append(missingVars, varName)
		}
	}
	if len(missingVars) > 0 {
		return fmt.Errorf("Missing environment variables %s", missingVars)
	}
	return nil
}

// getToken gets a token using your credentials. The token will be used by clients.
func getToken(credentials map[string]string) (*azure.ServicePrincipalToken, error) {
	oauthConfig, err := azure.PublicCloud.OAuthConfigForTenant(credentials["AZURE_TENANT_ID"])
	if err != nil {
		return nil, err
	}
	token, err := azure.NewServicePrincipalToken(*oauthConfig, credentials["AZURE_CLIENT_ID"], credentials["AZURE_CLIENT_SECRET"], azure.PublicCloud.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}
	return token, nil
}

// createNeededResources creates all common resources needed before creating VMs.
func createNeededResources() (*network.Subnet, error) {
	fmt.Println("Create resource group...")
	resourceGroupParameters := resources.ResourceGroup{
		Location: &location}
	if _, err := groupClient.CreateOrUpdate(resourceGroupName, resourceGroupParameters); err != nil {
		return nil, err
	}

	fmt.Println("Create storage account...")
	accountParameters := storage.AccountCreateParameters{
		Sku: &storage.Sku{
			Name: storage.StandardLRS},
		Location:   &location,
		Properties: &storage.AccountPropertiesCreateParameters{}}
	if _, err := accountClient.Create(resourceGroupName, accountName, accountParameters, nil); err != nil {
		return nil, err
	}

	fmt.Println("Create virtual network...")
	vNetName := "vNet"
	vNetParameters := network.VirtualNetwork{
		Location: &location,
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{
				AddressPrefixes: &[]string{"10.0.0.0/16"}}}}
	if _, err := vNetClient.CreateOrUpdate(resourceGroupName, vNetName, vNetParameters, nil); err != nil {
		return nil, err
	}

	fmt.Println("Create subnet...")
	subnetName := "subnet"
	subnet := network.Subnet{
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("10.0.0.0/24")}}
	if _, err := subnetClient.CreateOrUpdate(resourceGroupName, vNetName, subnetName, subnet, nil); err != nil {
		return nil, err
	}

	fmt.Println("Get subnet info...")
	subnetInfo, err := subnetClient.Get(resourceGroupName, vNetName, subnetName, "")
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
	if _, err := vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil); err != nil {
		return err
	}

	fmt.Printf("Now you can connect to '%s' VM via 'ssh %s@%s' with password '%s'\n",
		vmName,
		*vm.Properties.OsProfile.AdminUsername,
		*publicIPaddress.Properties.DNSSettings.Fqdn,
		*vm.Properties.OsProfile.AdminPassword)

	return nil
}

// vmOperations performs simple VM operations.
func vmOperations(vmName string) error {
	fmt.Printf("Get VM '%s' by name...\n", vmName)
	vm, err := vmClient.Get(resourceGroupName, vmName, compute.InstanceView)
	if err != nil {
		return err
	}
	printVM(vm)

	fmt.Println("Tag VM...")
	vm.Tags = &(map[string]*string{
		"who rocks": to.StringPtr("golang"),
		"where":     to.StringPtr("on azure")})
	_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
	printError(err)

	fmt.Println("Attach data disk...")
	vm.Properties.StorageProfile.DataDisks = &[]compute.DataDisk{{
		Lun:  to.Int32Ptr(0),
		Name: to.StringPtr("dataDisk"),
		Vhd: &compute.VirtualHardDisk{
			URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, fmt.Sprintf("dataDisks-%v", vmName)))},
		CreateOption: compute.Empty,
		DiskSizeGB:   to.Int32Ptr(1)}}
	_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
	printError(err)

	fmt.Println("Detach data disks...")
	vm.Properties.StorageProfile.DataDisks = &[]compute.DataDisk{}
	_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
	printError(err)

	fmt.Println("Update OS disk size...")
	if vm.Properties.StorageProfile.OsDisk.DiskSizeGB == nil {
		vm.Properties.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
	}
	_, err = vmClient.Deallocate(resourceGroupName, vmName, nil)
	printError(err)
	if *vm.Properties.StorageProfile.OsDisk.DiskSizeGB <= 0 {
		*vm.Properties.StorageProfile.OsDisk.DiskSizeGB = 256
	}
	*vm.Properties.StorageProfile.OsDisk.DiskSizeGB += 10
	_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
	printError(err)

	fmt.Println("Start VM...")
	_, err = vmClient.Start(resourceGroupName, vmName, nil)
	printError(err)

	fmt.Println("Restart VM...")
	_, err = vmClient.Restart(resourceGroupName, vmName, nil)
	printError(err)

	fmt.Println("Stop VM...")
	_, err = vmClient.PowerOff(resourceGroupName, vmName, nil)
	printError(err)

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
		Properties: &network.PublicIPAddressPropertiesFormat{
			DNSSettings: &network.PublicIPAddressDNSSettings{
				DomainNameLabel: to.StringPtr(fmt.Sprintf("azuresample-%s", strings.ToLower(machine[:5])))}}}
	if _, err := addressClient.CreateOrUpdate(resourceGroupName, IPname, pipParameters, nil); err != nil {
		return nil, nil, err
	}

	fmt.Println("\tGet public IP address info...")
	publicIPaddress, err := addressClient.Get(resourceGroupName, IPname, "")
	if err != nil {
		return nil, nil, err
	}

	fmt.Println("\tCreate NIC...")
	nicName := fmt.Sprintf("nic-%s", machine)
	nicParameters := network.Interface{
		Location: &location,
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &[]network.InterfaceIPConfiguration{{
				Name: to.StringPtr(fmt.Sprintf("IPconfig-%s", machine)),
				Properties: &network.InterfaceIPConfigurationPropertiesFormat{
					PublicIPAddress:           &publicIPaddress,
					PrivateIPAllocationMethod: network.Dynamic,
					Subnet: subnetInfo}}}}}
	if _, err := interfacesClient.CreateOrUpdate(resourceGroupName, nicName, nicParameters, nil); err != nil {
		return &publicIPaddress, nil, err
	}

	fmt.Println("\tGet NIC info...")
	nicParameters, err = interfacesClient.Get(resourceGroupName, nicName, "")
	if err != nil {
		return &publicIPaddress, nil, err
	}

	return &publicIPaddress, &nicParameters, nil
}

// setVMparameters builds the VirtualMachine argument for creating or updating a VM.
func setVMparameters(vmName, publisher, offer, sku, nicID string) compute.VirtualMachine {
	return compute.VirtualMachine{
		Location: &location,
		Properties: &compute.VirtualMachineProperties{
			HardwareProfile: &compute.HardwareProfile{
				VMSize: compute.StandardDS1},
			StorageProfile: &compute.StorageProfile{
				ImageReference: &compute.ImageReference{
					Publisher: &publisher,
					Offer:     &offer,
					Sku:       &sku,
					Version:   to.StringPtr("latest")},
				OsDisk: &compute.OSDisk{
					Name: to.StringPtr("osDisk"),
					Vhd: &compute.VirtualHardDisk{
						URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, vmName))},
					CreateOption: compute.FromImage}},
			OsProfile: &compute.OSProfile{
				ComputerName:  &vmName,
				AdminUsername: to.StringPtr("notadmin"),
				AdminPassword: to.StringPtr("Pa$$w0rd1975")},
			NetworkProfile: &compute.NetworkProfile{
				NetworkInterfaces: &[]compute.NetworkInterfaceReference{{
					ID: &nicID,
					Properties: &compute.NetworkInterfaceReferenceProperties{
						Primary: to.BoolPtr(true)}}}}}}
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

// printError prints non nil errors.
func printError(err error) {
	if err != nil {
		fmt.Printf("Error: %s\n", err)
	}
}
