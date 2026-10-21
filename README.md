# AzureAutoHibernate

[![CI](https://github.com/smitstech/AzureAutoHibernate/actions/workflows/ci.yml/badge.svg)](https://github.com/smitstech/AzureAutoHibernate/actions/workflows/ci.yml)
[![CodeQL](https://github.com/smitstech/AzureAutoHibernate/actions/workflows/codeql.yml/badge.svg)](https://github.com/smitstech/AzureAutoHibernate/actions/workflows/codeql.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

Automatically hibernate Azure VMs when idle, using IMDS + Managed Identity.  
Runs as a lightweight Windows service with optional toast notifications.

---

## Downloads

Grab the latest binaries here:  
https://github.com/smitstech/AzureAutoHibernate/releases

---

## Features

- Idle detection with configurable thresholds
- Pre-hibernate toast notification
- Hibernate via Azure IMDS using Managed Identity
- Windows service + notifier app
- No Azure credentials stored locally
- Extremely lightweight and safe

---

## Why?

Dev/test VMs cost money when left running.  
AzureAutoHibernate shuts them down automatically when they're idle — similar to a laptop closing the lid.

---

# Quick Start

### 1. Enable Managed Identity

```bash
az vm identity assign --name YOUR_VM --resource-group YOUR_RG
```

### 2. Grant Hibernate Permission

```bash
PRINCIPAL_ID=$(az vm identity show --name YOUR_VM --resource-group YOUR_RG --query principalId -o tsv)

az role assignment create \
  --assignee $PRINCIPAL_ID \
  --role "Virtual Machine Contributor" \
  --scope "/subscriptions/YOUR_SUBSCRIPTION/resourceGroups/YOUR_RG/providers/Microsoft.Compute/virtualMachines/YOUR_VM"
```

### 3. Copy Files to VM

Place these in a folder (e.g., `C:\Program Files\AzureAutoHibernate\`):

- `AzureAutoHibernate.exe`
- `AzureAutoHibernate.Notifier.exe`
- `AzureAutoHibernate.Updater.exe`
- `config.json`

### 4. Install the Service

```cmd
AzureAutoHibernate.exe -install
sc create AzureAutoHibernate binPath= "C:\Program Files\AzureAutoHibernate\AzureAutoHibernate.exe" start= auto
sc start AzureAutoHibernate
```

### 5. Verify

Open **Event Viewer → Applications and Services Logs → AzureAutoHibernate**

---

# Features

- **Automatic VM Discovery** via Azure IMDS
- **Session Monitoring** for active + disconnected users
- **Keyboard/Mouse Activity Detection**
- **Three Idle Conditions** (configurable thresholds):

  - No users logged in
  - All users disconnected
  - Logged-in user inactive

- **User Warnings** via toast notifications

  - Configurable warning period
  - Shows time remaining + hibernation reason
  - Cancellable via user activity

- **Minimum Uptime Enforcement** (prevents boot→hibernate loops)
- **Azure Integration** (hibernate via Managed Identity)
- **Windows Service** with auto-startup
- **Dynamic Polling** for minimal overhead
- **Event Log Integration** with categorized event IDs
- **Flexible Logging** (`debug`, `info`, `warn`, `error`)
- **Auto-Update** (optional, checks GitHub releases)

---

# Prerequisites

- Windows VM running on Azure
- VM size must support **Hibernate**
- **System Managed Identity enabled**
- Managed Identity must have the hibernate action permission
- Go 1.21+ (only required if building from source)

> If IMDS is unreachable on your VM (e.g. a VPN or exit node intercepts
> link-local traffic), see
> [Service Principal Authentication](#service-principal-authentication-optional)
> for an alternative to managed identity.

---

# Configuration

Example `config.json`:

```json
{
  "noUsersIdleMinutes": 15,
  "allDisconnectedIdleMinutes": 15,
  "inactiveUserIdleMinutes": 30,
  "inactiveUserWarningMinutes": 5,
  "minimumUptimeMinutes": 5,
  "logLevel": "info",
  "autoUpdate": false,
  "updateCheckIntervalHr": 24
}
```

### Parameters

| Parameter                    | Description                                | Default |
| ---------------------------- | ------------------------------------------ | ------- |
| `noUsersIdleMinutes`         | Hibernate when _no users_ logged in        | 15      |
| `allDisconnectedIdleMinutes` | Hibernate when _all sessions disconnected_ | 15      |
| `inactiveUserIdleMinutes`    | Hibernate when _no input_ detected         | 30      |
| `inactiveUserWarningMinutes` | Warning countdown before hibernate         | 5       |
| `minimumUptimeMinutes`       | Minimum uptime after boot/resume           | 5       |
| `logLevel`                   | Logging verbosity                          | `info`  |
| `autoUpdate`                 | Enable automatic update checking           | `false` |
| `updateCheckIntervalHr`      | Hours between update checks                | 24      |

**Notes:**

- At least one idle condition must be > 0
- Warning period applies _only_ to inactive-user condition
- Auto-update downloads from GitHub releases and restarts the service automatically

### Service Principal Authentication (Optional)

By default the service authenticates with the VM's **system managed identity**
via the Azure Instance Metadata Service (IMDS at `169.254.169.254`). If IMDS is
unreachable — for example when a VPN or exit node intercepts link-local
traffic — you can instead authenticate with a **Service Principal** against the
public login endpoint (`login.microsoftonline.com`):

```json
{
  "tenantId": "00000000-0000-0000-0000-000000000000",
  "clientId": "00000000-0000-0000-0000-000000000000",
  "clientSecret": "your-client-secret",
  "subscriptionId": "00000000-0000-0000-0000-000000000000",
  "resourceGroup": "my-resource-group",
  "vmName": "my-vm"
}
```

| Parameter        | Description                                                                 | Required            |
| ---------------- | --------------------------------------------------------------------------- | ------------------- |
| `tenantId`       | Azure AD tenant of the Service Principal                                    | All three or none   |
| `clientId`       | Application (client) ID of the Service Principal                            | All three or none   |
| `clientSecret`   | Client secret of the Service Principal                                      | All three or none   |
| `subscriptionId` | VM subscription — used only when IMDS is unavailable                        | With SP auth        |
| `resourceGroup`  | VM resource group — used only when IMDS is unavailable                      | With SP auth        |
| `vmName`         | VM name — used only when IMDS is unavailable                                | With SP auth        |

**Notes:**

- `tenantId`, `clientId` and `clientSecret` must be set **together or not at
  all** — the configuration fails validation if only some are present
- `subscriptionId`, `resourceGroup` and `vmName` are ignored while IMDS is
  reachable; set them so the service can identify its VM when IMDS is not
- The secret is stored in **plain text** in `config.json`. Restrict access to
  the file, use a secret dedicated to this service, and rotate it if exposed

**Grant the Service Principal least privilege** — it only needs to read the VM
state and deallocate (hibernate) it. Create a custom role and scope the
assignment to this VM only:

```bash
az role definition create --role-definition '{
  "Name": "AzureAutoHibernate Operator",
  "Description": "Read VM state and hibernate (deallocate) a VM",
  "Actions": [
    "Microsoft.Compute/virtualMachines/read",
    "Microsoft.Compute/virtualMachines/deallocate/action"
  ],
  "AssignableScopes": ["/subscriptions/<subscription-id>"]
}'

az role assignment create \
  --assignee <clientId> \
  --role "AzureAutoHibernate Operator" \
  --scope "/subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.Compute/virtualMachines/<vm-name>"
```

---

# Building

### Using Build Scripts (Recommended)

**On Windows (PowerShell):**
```powershell
.\build.ps1
```

**On Linux/Mac (with Make):**
```bash
make build
```

Both methods automatically inject version information from git tags.

### Check Version

```cmd
AzureAutoHibernate.exe -version
```

Output:
```
v1.0.0
```

### Manual Build

To build manually without version injection:

**On Windows:**
```bash
go build -o AzureAutoHibernate.exe ./cmd/autohibernate
go build -ldflags="-H=windowsgui" -o AzureAutoHibernate.Notifier.exe ./cmd/notifier
```

**To cross-compile from Linux/Mac:**
```bash
GOOS=windows GOARCH=amd64 go build -o AzureAutoHibernate.exe ./cmd/autohibernate
GOOS=windows GOARCH=amd64 go build -ldflags="-H=windowsgui" -o AzureAutoHibernate.Notifier.exe ./cmd/notifier
```

**Executables:**

- `AzureAutoHibernate.exe` — the SYSTEM Windows service
- `AzureAutoHibernate.Notifier.exe` — per-session notifier UI
- `AzureAutoHibernate.Updater.exe` — update helper (applies updates after service stops)

---

# Testing

### Debug Mode

```cmd
AzureAutoHibernate.exe -debug
```

### Run Tests

```bash
go test ./...
```

### View Logs

Event Viewer → **AzureAutoHibernate**

---

# Updates

### Check for Updates Manually

```cmd
AzureAutoHibernate.exe -check-update
```

This will check GitHub releases and report if a newer version is available.

### Automatic Updates

Set `autoUpdate: true` in `config.json` to enable automatic updates. The service will:

1. Check for updates at the configured interval (default: every 24 hours)
2. Download new versions from GitHub releases
3. Spawn the updater helper process
4. Updater stops the service reliably (with retries and 10-minute timeout)
5. Replace executable files (config.json is merged, not replaced)
6. Restart the service automatically

**Safe & Reliable:**
- **User settings preserved**: Your `config.json` preferences are kept, new fields added automatically
- **Fails safely**: Update won't proceed if service won't stop (prevents broken updates)
- **Progress logging**: Updates logged to `%TEMP%\AzureAutoHibernate.Updater.log`
- **Version verification**: Check Windows Event Log after restart to confirm version

**Note:** The updater process runs with the same permissions as the service (SYSTEM).

---

# How It Works

### Startup

- Loads configuration
- Retrieves VM metadata via IMDS
- Validates Managed Identity and capabilities

### Dynamic Polling

- Polls infrequently when far from thresholds
- Polls every 5 seconds during warning windows

### Idle Detection

- No Users → Immediate hibernate
- All Disconnected → Immediate hibernate
- Inactive User → Warning period → Hibernate

### Warning Phase

- Notifier displays toast notifications
- User movement cancels countdown instantly
- Notifications throttled to once every 30 seconds

### Hibernate Execution

- Gets token from IMDS
- Calls Azure Hibernate API
- VM hibernates preserving memory to disk

---

# Architecture

```
AzureAutoHibernate.exe (SYSTEM)
   ├─ IdleMonitor
   ├─ NotifierManager
   │     └─ AzureAutoHibernate.Notifier.exe (per session)
   ├─ AzureHibernateClient
   └─ UpdateChecker (optional)
         └─ AzureAutoHibernate.Updater.exe (on update)
```

### Notifier Architecture

```
Service (SYSTEM) → Named Pipe (JSON) → AzureAutoHibernate.Notifier.exe (User Session)
```

Session-0 isolation requires this two-process design.

---

# Troubleshooting

### Notifications Not Appearing

- Make sure Windows Notifications are enabled
- Ensure `AzureAutoHibernate.Notifier.exe` is present
- Confirm service runs as SYSTEM
- Check Event Log for `ERROR_NOT_ALL_ASSIGNED`

### VM Doesn't Hibernate

- Confirm Azure Hibernate is enabled for VM size
- Check Managed Identity permissions
- Look for Azure API errors in Event Log

### Service Exits Immediately

- IMDS blocked or unreachable
- VM not running in Azure

---

# Security Considerations

- Runs as **LocalSystem** to access session info
- Uses **Managed Identity**, no secrets stored
- Metadata retrieved at runtime via IMDS
- Access tokens never persisted
- Automatically exits if not running on Azure

---

# Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md)

---

# License

MIT
