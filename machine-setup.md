# Machine Setup Guide

## Prerequisites for SSH Deployment

Before a machine can be deployed as a worker node via the casos UI, its SSH server must be configured to accept password-based root login.

### WSL / Ubuntu Target Node

1. **Install the SSH server**

   ```bash
   sudo apt update && sudo apt install -y openssh-server
   ```

2. **Start the SSH service**

   ```bash
   sudo service ssh start
   ```

3. **Set the root password**

   ```bash
   sudo passwd root
   ```

4. **Enable root login and password authentication**

   ```bash
   sudo sed -i 's/#PermitRootLogin prohibit-password/PermitRootLogin yes/' /etc/ssh/sshd_config
   sudo sed -i 's/#PasswordAuthentication yes/PasswordAuthentication yes/' /etc/ssh/sshd_config
   sudo service ssh restart
   ```

5. **Verify connectivity**

   From another machine (or Windows PowerShell):

   ```
   ssh root@<machine-ip> -p 22
   ```

   You should be prompted for the root password and successfully log in.

### Machine Record in casos

When adding the machine in the casos UI, set the fields as follows:

| Field     | Value                        |
|-----------|------------------------------|
| IP        | WSL host IP (e.g. `172.22.149.109`) |
| Port      | `22`                         |
| Username  | `root`                       |
| Auth Type | `password`                   |
| Password  | the root password you set above |

Once connectivity is confirmed, click **Deploy Node** to begin worker node deployment.
