---
services: virtual-machines
platforms: go
author: mcardosos
---

#Azure Virtual Machine Management Sample using Azure SDK for Go

This package demonstrates how to manage your Azure virtual machine (VM) using Go, and specifically how to:
- Create a virtual machine
- Tag a virtual machine
- Attach and detach data disks
- Update the OS disk size
- Start, restart and stop a virtual machine
- List virtual machines
- Delete a virtual machine

If you don't have a Microsoft Azure subscription you can get a FREE trial account [here](https://azure.microsoft.com/pricing/free-trial).

**On this page**

- [Run this sample](#run)
- [What does VMsample.go do?](#sample)
- [More information](#info)

<a id="run"></a>
## Run this sample

1. Create a [service principal](https://azure.microsoft.com/documentation/articles/resource-group-authenticate-service-principal-cli/). You will need the Tenant ID, Client ID and Client Secret for [authentication](https://github.com/Azure/azure-sdk-for-go/tree/master/arm#first-a-sidenote-authentication-and-the-azure-resource-manager), so keep them as soon as you get them.
2. Get your Azure Subscription ID using either of the methods mentioned below:
  - Get it through the [portal](portal.azure.com) in the subscriptions section.
  - Get it using the [Azure CLI](https://azure.microsoft.com/documentation/articles/xplat-cli-install/) with command `azure account show`.
  - Get it using [Azure Powershell](https://azure.microsoft.com/documentation/articles/powershell-install-configure/) whit cmdlet `Get-AzureRmSubscription`.
3. Set environment variables `AZURE_TENANT_ID = <TENANT_ID>`, `AZURE_CLIENT_ID = <CLIENT_ID>`, `AZURE_CLIENT_SECRET = <CLIENT_SECRET>` and `AZURE_SUBSCRIPTION_ID = <SUBSCRIPTION_ID>`.
4. Get this sample using command `go get -u github.com/Azure-Samples/compute-go-manage-vm`.
5. Get the [Azure SDK for Go](https://github.com/Azure/azure-sdk-for-go) using command `go get -u github.com/Azure/azure-sdk-for-go`. Or in case that you want to vendor your dependencies using [glide](https://github.com/Masterminds/glide), navigate to this sample's directory and use command `glide install`.
6. Compile and run the sample.

<a id="sample"></a>
## What does VMsample.go do?

First, all resources needed before creating a VM are created (resource group, storage account, virtual network, subnet)

### Create a Linux VM

```go
publicIPaddress, nicParameters, err := createPIPandNIC(linuxVMname, subnetInfo)
vm := setVMparameters(linuxVMname, "Canonical", "UbuntuServer", "16.04.0-LTS", *nicParameters.ID)
if _, err := vmClient.CreateOrUpdate(resourceGroupName, linuxVMname, vm, nil); err != nil {
	return err
}
```

### Get the VM properties

```go
vm, err := vmClient.Get(resourceGroupName, vmName, compute.InstanceView)
```

### Tag the VM

```go
vm.Tags = &(map[string]*string{
	"who rocks": to.StringPtr("golang"),
	"where":     to.StringPtr("on azure")})
_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
```

### Attach and detach data disks to the VM

```go
fmt.Println("Attach data disk...")
vm.Properties.StorageProfile.DataDisks = &[]compute.DataDisk{{
	Lun:  to.Int32Ptr(0),
	Name: to.StringPtr("dataDisk"),
	Vhd: &compute.VirtualHardDisk{
		URI: to.StringPtr(fmt.Sprintf(vhdURItemplate, accountName, fmt.Sprintf("dataDisks-%v", vmName)))},
	CreateOption: compute.Empty,
	DiskSizeGB:   to.Int32Ptr(1)}}
_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)

fmt.Println("Detach data disks...")
vm.Properties.StorageProfile.DataDisks = &[]compute.DataDisk{}
_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
```

### Updates the VM's OS disk size

```go
if vm.Properties.StorageProfile.OsDisk.DiskSizeGB == nil {
	vm.Properties.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
}
_, err = vmClient.Deallocate(resourceGroupName, vmName, nil)
if *vm.Properties.StorageProfile.OsDisk.DiskSizeGB <= 0 {
	*vm.Properties.StorageProfile.OsDisk.DiskSizeGB = 256
}
*vm.Properties.StorageProfile.OsDisk.DiskSizeGB += 10
_, err = vmClient.CreateOrUpdate(resourceGroupName, vmName, vm, nil)
```

### Starts, restarts and stops the VM

```go
_, err = vmClient.Start(resourceGroupName, vmName, nil)
_, err = vmClient.Restart(resourceGroupName, vmName, nil)
_, err = vmClient.PowerOff(resourceGroupName, vmName, nil)

```

# Creates a Windows VM

```go
publicIPaddress, nicParameters, err := createPIPandNIC(windowsVMname, subnetInfo)
vm := setVMparameters(windowsVMname, "MicrosoftWindowsServerEssentials", "WindowsServerEssentials", "WindowsServerEssentials", *nicParameters.ID)
if _, err := vmClient.CreateOrUpdate(resourceGroupName, windowsVMname, vm, nil); err != nil {
	return err
}
```

# Lists all the VMs in your subscription.

```go
vmList, err := vmClient.ListAll()
```

### Delete the Linux and the Windows VM

```go
_, err = vmClient.Delete(resourceGroupName, linuxVMname, nil)
_, err = vmClient.Delete(resourceGroupName, windowsVMname, nil)

```

<a id="info"></a>
## More information

- [First a Sidenote: Authentication and the Azure Resource Manager](https://github.com/Azure/azure-sdk-for-go/tree/master/arm#first-a-sidenote-authentication-and-the-azure-resource-manager)
- [Azure Virtual Machines documentation](https://azure.microsoft.com/services/virtual-machines/)
- [Learning Path for Virtual Machines](https://azure.microsoft.com/documentation/learning-paths/virtual-machines/)

***

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.