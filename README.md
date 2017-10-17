---
services: virtual-machines
platforms: go
author: mcardosos
---

# Azure Virtual Machine Management Sample using Azure SDK for Go

This sample demonstrates how to manage your Azure virtual machine (VM) using Go, and specifically how to:

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
- [What does example.go do?](#sample)
- [More information](#info)

<a id="run"></a>

## Run this sample

1. If you don't already have it, [install Go 1.7](https://golang.org/dl/).

1. Clone the repository.

    ```
    git clone https://github.com:Azure-Samples/virtual-machines-go-manage.git
    ```

1. Install the dependencies using glide.

    ```
    cd virtual-machines-go-manage
    glide install
    ```

1. Create an Azure service principal either through
    [Azure CLI](https://azure.microsoft.com/documentation/articles/resource-group-authenticate-service-principal-cli/),
    [PowerShell](https://azure.microsoft.com/documentation/articles/resource-group-authenticate-service-principal/)
    or [the portal](https://azure.microsoft.com/documentation/articles/resource-group-create-service-principal-portal/).

1. Set the following environment variables using the information from the service principle that you created.

    ```
    export AZURE_TENANT_ID={your tenant id}
    export AZURE_CLIENT_ID={your client id}
    export AZURE_CLIENT_SECRET={your client secret}
    export AZURE_SUBSCRIPTION_ID={your subscription id}
    ```

    > [AZURE.NOTE] On Windows, use `set` instead of `export`.

1. Run the sample.

    ```
    go run example.go
    ```

<a id="sample"></a>

## What does example.go do?

First, it creates all resources needed before creating a VM (resource group, storage account, virtual network, subnet)

### Create a VM

```go
publicIPaddress, nicParameters := createPIPandNIC(vmName, subnetInfo)
vm := setVMparameters(vmName, "Canonical", "UbuntuServer", "16.04.0-LTS", *nicParameters.ID)
_, errChan := vmClient.CreateOrUpdate(groupName, vmName, vm, nil)
```

### Get the VM properties

```go
vm, err := vmClient.Get(groupName, vmName, compute.InstanceView)
```

### Tag the VM

```go
vm.Tags = &(map[string]*string{
	"who rocks": to.StringPtr("golang"),
	"where":     to.StringPtr("on azure"),
	})
_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Attach data disks to the VM

```go
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
```

### Detach data disks

```go
vm.StorageProfile.DataDisks = &[]compute.DataDisk{}
_, errChan := vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Updates the VM's OS disk size

```go
if vm.StorageProfile.OsDisk.DiskSizeGB == nil {
	vm.StorageProfile.OsDisk.DiskSizeGB = to.Int32Ptr(0)
}
_, errChan := vmClient.Deallocate(groupName, vmName, nil)
if *vm.StorageProfile.OsDisk.DiskSizeGB <= 0 {
	*vm.StorageProfile.OsDisk.DiskSizeGB = 256
}
*vm.StorageProfile.OsDisk.DiskSizeGB += 10
_, errChan = vmClient.CreateOrUpdate(groupName, vmName, *vm, nil)
```

### Starts, restarts and stops the VM

```go
_, errChan := vmClient.Start(groupName, vmName, nil)
_, errChan = vmClient.Restart(groupName, vmName, nil)
_, errChan = vmClient.PowerOff(groupName, vmName, nil)
```

# Lists all the VMs in your subscription.

```go
vmList, err := vmClient.ListAll()
```

### Delete the VMs and other resources

```go
_, errChan := vmClient.Delete(groupName, linuxVMname, nil)
_, errChan = vmClient.Delete(groupName, windowsVMname, nil)

_, errChan = groupClient.Delete(groupName, nil)
```

<a id="info"></a>

## More information

- [Windows Virtual Machines documentation](https://azure.microsoft.com/documentation/services/virtual-machines/windows/)
- [Linux Virtual Machines documentation](https://azure.microsoft.com/documentation/services/virtual-machines/linux/)

***

This project has adopted the [Microsoft Open Source Code of Conduct](https://opensource.microsoft.com/codeofconduct/). For more information see the [Code of Conduct FAQ](https://opensource.microsoft.com/codeofconduct/faq/) or contact [opencode@microsoft.com](mailto:opencode@microsoft.com) with any additional questions or comments.