import requests
import time
import json
import logging
import sys, os

logger = logging.getLogger('logger')
if os.getenv("DEBUG") != None:
    logger.setLevel(logging.DEBUG)
else:
    logger.setLevel(logging.INFO)
logger.addHandler(logging.StreamHandler(stream=sys.stdout))


PATH_FIP_JSON_DB = "/var/run/docker/emc_fip_db.json"
INTERVAL_SECOND = 2
FLOATING_IP_LABEL = "io.rancher.container.floating.ip"

class MetadataConfd:
    PREFIX = "http://rancher-metadata.rancher.internal/2015-07-25"
    URL_SELF_HOST_UUID = PREFIX + "/self/host/uuid"
    URL_SELF_HOST_IP = PREFIX + "/self/host/agent_ip"
    URL_CONTAINERS = PREFIX + "/containers"
    URL_CONTAINER_HOST_UUID = PREFIX + "/containers/%s/host_uuid"
    URL_CONTAINER_UUID = PREFIX + "/containers/%s/uuid"
    URL_CONTAINER_LABELS = PREFIX + "/containers/%s/labels"
    URL_CONTAINER_IP = PREFIX + "/containers/%s/labels/io.rancher.container.ip"
    URL_CONTAINER_FLOATING_IP = PREFIX + "/containers/%s/labels/io.rancher.container.floating.ip"

    def __init__(self, floating_ip_label, containers_orign):
        self.floating_ip_label = floating_ip_label
        self.my_host_uuid = self.get_self_host_uuid()
        self.my_host_ip = self.get_self_host_ip()
        self.containers_origin = containers_orign

    @staticmethod
    def get_value(url):
        try:
            r = requests.get(url)
        except Exception as e:
            logger.debug("Exception: get_value(%s)" % url)
            print e
            return None

        if r.status_code != requests.codes.ok:
            return None
        return r.text

    def get_self_host_uuid(self):
        resp = self.get_value(self.URL_SELF_HOST_UUID)
        logger.debug("Get host uuid %s" % resp)
        return resp

    def get_self_host_ip(self):
        resp = self.get_value(self.URL_SELF_HOST_IP)
        logger.debug("Get host ip %s" % resp)
        return resp

    def get_containers(self):
        resp = self.get_value(self.URL_CONTAINERS)
        '''' ## format ##
        0=Network+Agent
        1=jenkins2-agent_swarm-clients_2
        2=jenkins2-agent_swarm-clients_3
        '''
        items = resp.split('\n')
        containers = dict()
        for i in items:
            kv = i.split('=')
            if len(kv) == 2:
                containers[kv[0]] = kv[1]
        logger.debug("Get containers %s" % str(containers))
        return containers

    def get_host_uuid_by_container(self, container_name):
        resp = self.get_value(self.URL_CONTAINER_HOST_UUID % container_name)
        return resp

    def get_container_uuid_by_name(self, container_name):
        resp = self.get_value(self.URL_CONTAINER_UUID % container_name)
        return resp

    def get_container_ip_by_name(self, container_name):
        resp = self.get_value(self.URL_CONTAINER_IP % container_name)
        return resp

    def get_container_floating_ip_by_name(self, container_name):
        queryUrl = self.URL_CONTAINER_LABELS % container_name
        logger.debug("Get containers float ip query: %s" % queryUrl)
        resp = self.get_value(queryUrl)
        items = resp.split('\n')
        if self.floating_ip_label not in items:
            return None
        else:
            resp = self.get_value(self.URL_CONTAINER_FLOATING_IP % container_name)
            return resp

    def get_float_ip_containers_on_my_host(self):
        containers_on_my_host = dict()
        containers = self.get_containers()
        for k in containers:
            name = containers[k]
            if self.get_host_uuid_by_container(name) == self.my_host_uuid:
                uuid = self.get_container_uuid_by_name(name)
                ip = self.get_container_ip_by_name(name)
                floating_ip = self.get_container_floating_ip_by_name(name)
                if (uuid is None) or (ip is None) or (floating_ip is None):
                    continue
                containers_on_my_host[name] = {
                    "uuid": uuid,
                    "name": name,
                    "managed_ip": ip,
                    "floating_ip": floating_ip,
                }
        logger.debug("Get containers on my host %s" % str(containers_on_my_host))
        return containers_on_my_host

    def get_containers_need_to_update(self):
        containers_added = dict()
        containers_removed = dict()
        containers_updated = dict()
        containers_with_floating_ip = self.get_float_ip_containers_on_my_host()
       
        logger.debug("current host float ip status %s" % str(containers_with_floating_ip))
        
        for k in self.containers_origin:
            if k not in containers_with_floating_ip:
                logger.debug("%s need remove from this host" % str(k))
                containers_removed[k] = self.containers_origin[k]
        for k in containers_with_floating_ip:
            if k not in self.containers_origin:
                logger.debug("%s need add to this host" % str(k))
                containers_added[k] = containers_with_floating_ip[k]
        for k in self.containers_origin:
            if containers_with_floating_ip.has_key(k):
                if self.containers_origin[k]['floating_ip'] != containers_with_floating_ip[k]['floating_ip']:
                    containers_added[k] = containers_with_floating_ip[k]
                    containers_removed[k] = self.containers_origin[k]

        self.containers_origin = containers_with_floating_ip
        return containers_added, containers_removed


####################################################################
#
# MAIN BODY BEGIN
#
####################################################################

def main():
    
    # wait dns set up
    while True:
        hostname = "rancher-metadata.rancher.internal"
        response = os.system("ping -c 1 " + hostname)
        if response == 0 :
            break
        
        logger.info("Retry reslover domain name `rancher-metadata.rancher.internal`")
        time.sleep(5)

    containers_with_fip = dict()
    md_confd = MetadataConfd(FLOATING_IP_LABEL, containers_with_fip)

    while True:
        try:
            containers_added, containers_removed = \
                md_confd.get_containers_need_to_update()
        except Exception as e:
            raise e
            time.sleep(INTERVAL_SECOND)
            continue

        # process removed containers with floating ip
        for name in containers_removed:
            fip = containers_removed[name]['floating_ip']
            lip = containers_removed[name]['managed_ip']
            logger.info("Unbind floating ip for %s: %s to %s" % (name, fip, lip))
            containers_with_fip.pop(name)

        # process new added containers with floating ip
        for name in containers_added:
            fip = containers_added[name]['floating_ip']
            lip = containers_added[name]['managed_ip']
            logger.info("Bind floating ip for %s: %s to %s" % (name, fip, lip))
            containers_with_fip[name] = containers_added[name]
        
        logger.debug("Store work to json db: %s" % str(containers_with_fip))
        with open(PATH_FIP_JSON_DB, 'w+') as fp:
            json.dump(containers_with_fip, fp)

        time.sleep(INTERVAL_SECOND)


if __name__ == '__main__':
    main()
