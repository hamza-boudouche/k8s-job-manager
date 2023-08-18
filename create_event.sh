#!/bin/bash

API_ENDPOINT="https://kubernetes.default.svc.cluster.local/api/v1/namespaces/scheduler/events"

EVENT_DATA='{
  "apiVersion": "v1",
  "kind": "Event",
  "metadata": {
    "name": ""
  },
  "involvedObject": {
    "apiVersion": "batch/v1",
    "kind": "Job",
    "name": "",
    "namespace": "scheduler"
  },
  "reason": "JobCompleted",
  "message": "Job has been completed",
  "type": "Normal"
}'

EVENT_DATA=$(echo EVENT_DATA | jq -r --arg eventname "$1" --arg jobname "$2" '.metadata.name=$eventname | .involvedObject.name=$jobname')

curl -v -X POST $API_ENDPOINT \
  -H "Content-Type: application/json" \
  -d "$EVENT_DATA" \
  --header "Authorization: Bearer $(cat /var/run/secrets/kubernetes.io/serviceaccount/token)" \
  --cacert /var/run/secrets/kubernetes.io/serviceaccount/ca.crt

