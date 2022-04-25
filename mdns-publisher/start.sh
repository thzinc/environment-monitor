#!/bin/bash

# This takes the ENVRIONMENT_MONITOR_HOSTNAME environment variable specified on the balena dashboard or other method and uses mdns-publish-cname to create a CNAME record on mDNS.
# However, it sets a default of air.local if nothing is provided.

if [[ -z "$ENVRIONMENT_MONITOR_HOSTNAME" ]]; then
  mdns-publish-cname air.local
else
  mdns-publish-cname "$ENVRIONMENT_MONITOR_HOSTNAME".local
fi
