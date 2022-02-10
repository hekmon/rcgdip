# rcgdip

RClone GDrive inotify for Plex is a rclone companion for your plex server using a rclone mount on Google Drive. It will monitor changes on GDrive and launch targeted scan on your matching Plex libraries to automatically discover new files.

## Installation

### Get the software

#### From official releases

Download a binary from the [releases page](https://github.com/hekmon/rcgdip/releases).

#### From source

```bash
go build -ldflags='-s -w' -trimpath -o rcgdip github.com/hekmon/rcgdip/cmd
```

### Configure your system

While technically rcgdip can run anywhere, it is recommended to run it alongside your plex server and your rclone mount.

#### Execution environment

```bash
sudo useradd --home-dir /var/lib/rcgdip --create-home --system --shell /usr/sbin/nologin rcgdip
sudo chown rcgdip: /var/lib/rcgdip
sudo chmod 750 /var/lib/rcgdip
sudo wget https://github.com/hekmon/rcgdip/releases/download/v0.1.0/rcgdip_linux_amd64 -O /usr/local/bin/rcgdip
sudo chmod +x /usr/local/bin/rcgdip
# adapt to the group of your rclone config file, here the rclone config file is owned (and readable) by the rclone group
sudo usermod -a -G rclone rcgdip
```

#### systemd service

If your rclone mount is started with systemd too (for example using [rclonemount](https://github.com/hekmon/rclonemount)), add it to `After=` and `BindsTo=` as well. Eg:

```systemd
After=network-online.target plexmediaserver.service rclonemount@<yourinstance>.service
BindsTo=plexmediaserver.service rclonemount@<yourinstance>.service
```

##### mono instance

```bash
cat <<EOF | sudo tee /etc/systemd/system/rcgdip.service
[Unit]
Description=RClone GDrive inotify for Plex
Requires=network-online.target
After=network-online.target plexmediaserver.service
BindsTo=plexmediaserver.service

[Service]
Type=notify
User=rcgdip
WorkingDirectory=~
EnvironmentFile=/etc/default/rcgdip
ExecStart=/usr/local/bin/rcgdip
ExecReload=/bin/kill -SIGHUP $MAINPID
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
```

If your rclone mount is started with systemd too (for example using [rclonemount](https://github.com/hekmon/rclonemount)), add it to `After=` and `BindsTo=` as well. Eg:

```systemd
After=network-online.target plexmediaserver.service rclonemount@<yourinstance>.service
BindsTo=plexmediaserver.service rclonemount@<yourinstance>.service
```

##### multi instances

Optional. If you intend to run multiples instances.

```bash
cat <<EOF | sudo tee /etc/systemd/system/rcgdip@.service
[Unit]
Description=RClone GDrive inotify for Plex - %i
Requires=network-online.target
After=network-online.target plexmediaserver.service
BindsTo=plexmediaserver.service

[Service]
Type=notify
User=rcgdip
WorkingDirectory=~
EnvironmentFile=/etc/default/rcgdip_%i
ExecStart=/usr/local/bin/rcgdip -instance %i
ExecReload=/bin/kill -SIGHUP $MAINPID
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF
sudo systemctl daemon-reload
```

### Configure rcgdip

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

# Optionnal
RCGDIP_RCLONE_BACKEND_CRYPT_NAME=""
RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL=""
RCGDIP_LOGLEVEL="DEBUG"
EOF
sudo chown root:rcgdip "$confFile"
sudo chmod 640 "$confFile"
```

#### Multi instances

Same as mono instance but for the `instanceName` instance, change `confFile="/etc/default/rcgdip` to `confFile="/etc/default/rcgdip_instanceName`

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

3 values rclone values are importants to have rcgdip works as intended:

* `--attr-timeout` it is the time the FUSE mount is allowed to cache the informations before asking them again to rclone, keep the default unless you know what you are doing but it should lower than...
* ... `--dir-cache-time` which is the time rclone keeps the metadata of each folder without reasking the backend to answers the FUSE requests when `--attr-timeout` is elapsed. Also keep default if you can.
* `--poll-interval` the frequency used by rclone to ask the backend for changes, it allows for targetted updates of the dir cache even if it is still within the `--dir-cache-time` window. It should be lower than `--dir-cache-time`. Default value is fine here too.

rcgdip bases its prediction on the `--poll-interval` using the same default as rclone, if you customize the rclone `--poll-interval` for your rclone mount remeber to set the exact same value in `RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL` as well as rcgdip will wait this interval between the change event timestamp and the plex scan in order to be sure the local rclone mount had the time to discover the new file(s).

## Known issues

### Deletion events

It seems that while `--poll-interval` works very well for new files and file changes but it does not work for deleted files (it is actually tricky to support as you have to build and maintain your own index locally, which rcgdip does). It means that a new file will be seen by your rclone mount fairly quickly (respecting the `--poll-interval`) but deleted files will only disappears locally when rclone dir cache is expired (the `--dir-cache-time` flag). Because rcgdip waits for `--poll-interval` by default before launching a scan, a deleted event will trigger scan while the local rclone mount still sees it.
If this is an issue for you, please set the `RCGDIP_RCLONE_BACKEND_DRIVE_POLLINTERVAL` to the `--dir-cache-time` value. New files will appears for an extra delay but deleted files will be correctly be handled.

## Sponsoring

If you like rcgdip, please consider sponsoring [rclone](https://github.com/rclone/rclone) directly.
