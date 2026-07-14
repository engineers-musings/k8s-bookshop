#!/bin/sh
# A controller is a loop. Observe desired state, make the world match it, report status.
# This one is namespace-scoped on purpose: it holds a Role, not a ClusterRole, so it can
# only see the namespace it runs in. `kubectl get shelves -A` here would be Forbidden.
while true; do
  kubectl get shelves \
    -o jsonpath='{range .items[*]}{.metadata.name}{" "}{.spec.isbns[*]}{"\n"}{end}' |
  while read -r name isbns; do
    [ -z "$name" ] && continue
    count=$(echo "$isbns" | wc -w | tr -d ' ')

    # ACT: make the world match the spec. create|apply is the idiom that makes this
    # idempotent — a plain `create` fails on the second pass, and a reconcile loop
    # cannot afford to fail on the second pass.
    kubectl create configmap "shelf-$name" --from-literal=count="$count" \
      --dry-run=client -o yaml | kubectl apply -f - >/dev/null

    # REPORT: write what we observed to the status subresource.
    kubectl patch shelf "$name" --subresource=status --type=merge \
      -p "{\"status\":{\"bookCount\":$count}}" >/dev/null

    echo "reconciled shelf/$name -> bookCount=$count"
  done
  sleep 5
done
