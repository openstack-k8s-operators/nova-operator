package util

import (
        "fmt"
        "regexp"
        "strings"

        corev1 "k8s.io/api/core/v1"
)

// Create hostAliases from added /etc/hosts file of the OSP environment.
// All entries inside the ANSIBLE MANAGED BLOCK gets added. Important is that
// the /etc/hosts file in the config map has opening and closing tag.
func CreateOspHostsEntries(commonConfigMap *corev1.ConfigMap) ([]corev1.HostAlias, error){
        hostAliases := []corev1.HostAlias{}

        hostsFile, isset := commonConfigMap.Data["hosts"]

        if isset {
                re := regexp.MustCompile(`(?s).*BEGIN ANSIBLE MANAGED BLOCK\n(.*)# END ANSIBLE MANAGED BLOCK.*`)
                hostsEntries := re.FindStringSubmatch(hostsFile)
               
                if len(hostsEntries)>=1 {
                        for _, hostRecord := range strings.Split(hostsEntries[1], "\n") {
                                if len(hostRecord) > 0 {
                                        var ip string
                                        var names []string

                                        for i, r := range strings.Fields(hostRecord) {
                                                if i == 0 {
                                                        ip = r
                                                } else {
                                                        names = append(names, r)
                                                }
                                        }

                                        hostAlias := corev1.HostAlias{
                                                IP: ip,
                                                Hostnames: names,
                                        }
                                        hostAliases = append(hostAliases, hostAlias)
                                }
                        }
                } else {
                        return nil, fmt.Errorf("Ansible tags not found in hosts file of common-config map!")
                }
        } else {
                return nil, fmt.Errorf("No hosts file in common-config map!")
        }

        return hostAliases, nil
}
