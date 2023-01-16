create secrete for ssh key and configmap for playbook

oc create secret generic edpm-ssh-key  --type'kubernetes.io/ssh-auth' --from-file=ssh_private_key=/home/centos/.ssh/id_ed25519_oks --from-file=ssh_public_key=/home/centos/.ssh/id_ed25519_oks.pub
oc create configmap nova-playbooks --from-file=./playbooks/
