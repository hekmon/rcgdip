# rcgdip

RClone GDrive Inotify for Plex is a rclone companion for your plex server using a rclone mount on Google Drive. It will monitor changes on GDrive and launch targeted scans on your matching Plex libraries to automatically refresh your plex libraries as soon as your rclone drive mount can see the modifications.

It supports (directly from your rclone config file):

* [x] GDrive backend (scope `drive` only for now, `drive.file` support planned, see [GDrive scope](#gdrive-scope) for more details)
* [x] Custom root folder ID
* [x] Team Drives
* [x] Crypt backend on top of your GDrive backend for path decryption
* [x] OAuth2 token
* [ ] Service Account (planned)

- [rcgdip](#rcgdip)
  - [Installation](#installation)
    - [Get the software](#get-the-software)
      - [From official releases](#from-official-releases)
      - [From source](#from-source)
    - [Configure your system](#configure-your-system)
      - [Execution environment](#execution-environment)
      - [systemd service](#systemd-service)
        - [Mono instance](#mono-instance)
        - [Multi instances](#multi-instances)
    - [Configure rcgdip](#configure-rcgdip)
      - [Mono instance](#mono-instance-1)
      - [Multi instances](#multi-instances-1)
    - [Start rcgdip](#start-rcgdip)
      - [Mono instance](#mono-instance-2)
      - [Multi instances](#multi-instances-2)
  - [Things to consider](#things-to-consider)
    - [rclone mount config values](#rclone-mount-config-values)
    - [deletion events](#deletion-events)
    - [rclone version](#rclone-version)
    - [scan list optimizations](#scan-list-optimizations)
      - [same path optimization](#same-path-optimization)
      - [same ancester optimization](#same-ancester-optimization)
    - [GDrive scope](#gdrive-scope)
      - [change the original rclone config](#change-the-original-rclone-config)
      - [keep the original rclone config](#keep-the-original-rclone-config)
    - [db backup](#db-backup)
      - [Mono instance](#mono-instance-3)
      - [Multi instances](#multi-instances-3)
  - [Sponsoring](#sponsoring)

## Installation

### Get the software

#### From official releases

Download a binary from the [releases page](https://github.com/hekmon/rcgdip/releases).

#### From source

```bash
env GOOS=linux GOARCH=amd64 go build -ldflags="-s -w -X 'main.appVersion=$(git describe)'" -trimpath -o rcgdip github.com/hekmon/rcgdip/cmd
```

### Configure your system

While technically rcgdip can run anywhere, it is recommended to run it alongside your plex server and your rclone mount.

#### Execution environment

```bash
sudo useradd --home-dir '/var/lib/rcgdip' --create-home --system --shell '/usr/sbin/nologin' 'rcgdip'
sudo chown 'rcgdip:' '/var/lib/rcgdip'
sudo chmod 750 '/var/lib/rcgdip'
sudo wget 'https://github.com/hekmon/rcgdip/releases/download/v0.4.0/rcgdip_linux_amd64' -O '/usr/local/bin/rcgdip'
sudo chmod +x '/usr/local/bin/rcgdip'
# adapt to the group of your rclone config file, here the rclone config file is owned (and readable) by the rclone group
sudo usermod -a -G 'rclone' 'rcgdip'
```

#### systemd service

If your rclone mount is started with systemd too (for example using [rclonemount](https://github.com/hekmon/rclonemount)), add it to `After=` and `BindsTo=` as well. Eg:

```ini
[Unit]
After=rclonemount@<yourinstance>.service
BindsTo=rclonemount@<yourinstance>.service
```

Add it add while `systemd edit ...` on the next parts.

##### Mono instance

```bash
sudo wget 'https://raw.githubusercontent.com/hekmon/rcgdip/v0.4.0/systemd/rcgdip.service' -O '/etc/systemd/system/rcgdip.service'
sudo systemctl edit rcgdip.service # add your rclonemount unit as stated previously
sudo systemctl daemon-reload # not needed if you did systemctl edit
```

##### Multi instances

Optional. If you intend to run multiples instances. Each instance must have a custom edit.

```bash
sudo wget 'https://raw.githubusercontent.com/hekmon/rcgdip/v0.4.0/systemd/rcgdip%40.service' -O '/etc/systemd/system/rcgdip@.service'
sudo systemctl edit rcgdip@instanceNameA.service # add the related rclonemount unit as stated previously
sudo systemctl edit rcgdip@instanceNameB.service # add the (other) related rclonemount unit as stated previously
# ... repeat for each instance (1 rclone mount == 1 rcgdip instance)
sudo systemctl daemon-reload # not needed if you did systemctl edit
```

### Configure rcgdip

Check the [plex documentation](https://support.plex.tv/articles/204059436-finding-an-authentication-token-x-plex-token/) to recover your Plex token.

#### Mono instance

Adapt the values to yours.

```bash
confFile="/etc/default/rcgdip"
cat <<EOF | sudo tee "$confFile"
# Mandatory
RCGDIP_RCLONE_CONFIG_PATH="/etc/rclone/instance.conf"
RCGDIP_RCLONE_BACKEND_DRIVE_NAME="DriveBackendName"
RCGDIP_RCLONE_MOUNT_PATH="/some/path/used/as/destination/for/rclone/mount"
RCGDIP_PLEX_URL="http://127.0.0.1:32400"
RCGDIP_PLEX_TOKEN="yourshere"

# Optional
RCGDIP_RCLONE_BACKEND_CRYPT_NAME=""
RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL=""
RCGDIP_RCLONE_BACKEND_DRIVE_DIRCACHETIME=""
RCGDIP_LOGLEVEL="DEBUG"
EOF
sudo chown root:rcgdip "$confFile"
sudo chmod 640 "$confFile"
```

#### Multi instances

Same as mono instance but for the `instanceName` instance, change `confFile="/etc/default/rcgdip"` to `confFile="/etc/default/rcgdip_instanceName"`.

### Start rcgdip

#### Mono instance

```bash
sudo systemctl enable --now rcgdip.service
sudo journalctl -f -u rcgdip.service
```

#### Multi instances

For an instance named `instanceName`.

```bash
sudo systemctl enable --now rcgdip@instanceName.service
sudo journalctl -f -u rcgdip@instanceName.service
```

## Things to consider

### rclone mount config values

3 rclone mount values are importants to have rcgdip work as intended:

* `--attr-timeout` it is the time the FUSE mount is allowed to cache the informations before asking them again to rclone, keep the default unless you know what you are doing but it should always be lower than...
* `--dir-cache-time` which is the time rclone itself keeps the metadata of each folder without reasking the backend to answers the FUSE requests when `--attr-timeout` is elapsed. Default is fine but I like to increase it to `15m`.
* `--poll-interval` the frequency used by rclone to ask the backend for changes, it allows for targetted updates of the dir cache even if it is still within the `--dir-cache-time` window. It should be lower than `--dir-cache-time`. Default value is fine here too but I like to increase it to `5m`.

rcgdip bases its scan delay prediction to detect new and changes files on the `--poll-interval` value. So if you customize the rclone `--poll-interval` for your rclone mount remember to set the exact same value in `RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL` as well as rcgdip will wait this interval between the change event timestamp and the plex scan in order to be sure the local rclone mount had the time to discover the new file(s). Note that this also applies to `--dir-cache-time` and `RCGDIP_RCLONE_BACKEND_DRIVE_DIRCACHETIME`, see next section for more details.

If you leave `RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL` and `RCGDIP_RCLONE_BACKEND_DRIVE_DIRCACHETIME` empty, rcgdip will use the same defaults as rclone.

### deletion events

It seems that while `--poll-interval` works very well in rclone mounting  gdrive for new files and file changes, it does not work for deleted files (it is actually tricky to support as you have to build and maintain your own index locally, which rcgdip does). It means that a new file will be seen by your rclone mount fairly quickly (respecting the `--poll-interval`) but deleted files will only disappears locally when rclone dir cache is expired (the `--dir-cache-time` flag).

This is why in rcgdip you can specify `RCGDIP_RCLONE_BACKEND_DRIVE_DIRCACHETIME` in addition to `RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL`: deletion events will wait the `--dir-cache-time` duration before starting a scan while new or changed files will only wait the `--poll-interval` allowing fast detection when this is possible while still correctly handling deletion events.

If not specified, both `RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL` and `RCGDIP_RCLONE_BACKEND_DRIVE_DIRCACHETIME` take exactly the same default as rclone, be sure to use the same rclone version as the version of rcgdip you are using has been built against ! (see next section).

### rclone version

rcgdip is built with original rclone parts directly (as libs or constant values source) so to avoid any unexpected behaviors you should always ensure your rclone mount is using the same version of rclone as rcgdip was built against. RClone version used by rcgdip is always mentioned on each [release](https://github.com/hekmon/rcgdip/releases) notes.

### scan list optimizations

To avoid too many scan requests to be fired, severals optimizations are made inside a batch between the raw changes from drive and the scan requests to plex.

#### same path optimization

If several files have changed within the same directoy, only one scan job will be created for this directory and the wait time will be determined by the most recent event to ensure all modifications are seen by the time of the scan. But it also means that if one of this changes is a deletion event, the wait time can be longer (see [deletion events](#deletion-events) for more details) even for new files.

#### same ancester optimization

If 2 paths are scheduled for scan but one of them is actually a parent of the other, only the parent will be kept as it will also scan the child. But what about wait time ? If the parent was to be scanned at T+2 but the child was to be scanned at T+3, this optimization will remove the scan job for the child but adapt the scan time of the parent to T+3 in order for all changes to be detected within the scan.

### GDrive scope

To work, rcgdip starts by indexing every file in the targeted drive in order to correctly process the changes event from the API (the deletion events can not be handled without an index). But the current method for doing this initial index will fail if the drive scope is `drive.file`. While this is the best scope for a rclone mount it actually prevent rcgdip from working as expected. I am currently thinking of ways to actually perform a working indexing with this restricted scope but in the meantime, to have rcgdip working properly, the scope needs to be `drive` and the oauth token must have been issued with that scope.

If you are actually using the `drive.file` scope and wants to have rcgdip working there is 2 ways of doing it.

#### change the original rclone config

Simply edit the rclone config and edit the scope. Once edited, issue a [rclone reconnect command](https://rclone.org/commands/rclone_config_reconnect/) to refresh the oauth token with the new scope.

#### keep the original rclone config

If you want to keep the original file in place, copy it elsewhere, edit the copy, issue the [rclone reconnect command](https://rclone.org/commands/rclone_config_reconnect/) to refresh the oauth token with the new scope using the new rclone config file and use this new file as target for rcgdip.

### db backup

Do not directly backup db files while rcgdip is running ! By default a backup is performed at each start. If you need to make a backup of the db while rcgdip is running, send the `USR1` signal to the process: it will perform a backup in the backup directory which you can safely access after backup is done.

If you are using the systemd integration a simple reload of the unit will send the signal for you (see sub sections).

#### Mono instance

db directory is `rcgdip_storage` in the current working directory (`/var/lib/rcgdip` if you followed the installation steps) and the backup directory is `rcgdip_storage_backup`. To start a backup while rcgdip is running just launch `systemctl reload rcgdip.service` and check the logs.

#### Multi instances

For an instance named `instanceName`, the db directory is `rcgdip_storage_instanceName` in the current working directory (`/var/lib/rcgdip` if you followed the installation steps) and the backup directory is `rcgdip_storage_instanceName_backup`. To start a backup while rcgdip is running just launch `systemctl reload rcgdip@instanceName.service` and check the logs.

## Sponsoring

If you like rcgdip, please consider sponsoring [rclone](https://github.com/rclone/rclone) directly.
