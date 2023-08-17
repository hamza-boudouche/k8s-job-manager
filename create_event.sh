#!/bin/bash

API_ENDPOINT="https://kubernetes.default.svc.cluster.local/api/v1/namespaces/scheduler/events"

EVENT_DATA=$'{
  "apiVersion": "v1",
  "kind": "Event",
  "metadata": {
    "name": "\''$1'\''"
  },
  "involvedObject": {
    "apiVersion": "batch/v1",
    "kind": "Job",
    "name": "test3-ls",
    "namespace": "scheduler"
  },
  "reason": "JobCompleted",
  "message": "Job has been completed",
  "type": "Normal"
}'

curl -v -X POST $API_ENDPOINT \
  -H "Content-Type: application/json" \
  -d "$EVENT_DATA" \
  --header "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt

