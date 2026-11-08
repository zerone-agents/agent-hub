#!/usr/bin/env python3
"""Exercise the synthetic memory HTTP contract against an isolated deployment.

Uses only newly generated users/lifecycles. Credentials are read from the
environment, never printed. The application's pre-provisioned dataset is kept.
"""
import argparse
import json
import os
import sys
import time
import uuid
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen


class SmokeFailure(Exception):
    pass


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--timeout", type=int, default=45, help="maximum reconciliation wait per step")
    args = parser.parse_args()
    if not 1 <= args.timeout <= 120:
        parser.error("--timeout must be between 1 and 120 seconds")
    required = ["HUB_MEMORY_URL", "HUB_MEMORY_TOKEN", "HUB_MEMORY_POLICY_VERSION"]
    if any(not os.environ.get(name) for name in required):
        parser.error("set HUB_MEMORY_URL, HUB_MEMORY_TOKEN and HUB_MEMORY_POLICY_VERSION for a synthetic deployment")
    base = os.environ["HUB_MEMORY_URL"].rstrip("/") + "/api/v1/memory"
    token = os.environ["HUB_MEMORY_TOKEN"]
    policy = os.environ["HUB_MEMORY_POLICY_VERSION"]
    run_id = uuid.uuid4().hex
    scopes = [("synthetic-" + run_id + "-a", str(uuid.uuid4())), ("synthetic-" + run_id + "-b", str(uuid.uuid4()))]
    created, cleaned, steps = [], [], []

    def call(method, path, scope, body=None, expected=200):
        headers = {"Authorization": "Bearer " + token, "X-Trace-Id": run_id, "Content-Type": "application/json"}
        if scope:
            headers.update({"X-Memory-User-Id": scope[0], "X-Memory-Lifecycle-Id": scope[1]})
        req = Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method=method)
        try:
            with urlopen(req, timeout=10) as response:
                status, raw = response.status, response.read(2 << 20)
        except HTTPError as error:
            status, raw = error.code, error.read(2 << 20)
        except (URLError, TimeoutError):
            raise SmokeFailure("transport failure during " + method + " " + path) from None
        try:
            result = json.loads(raw)
        except (ValueError, UnicodeDecodeError):
            raise SmokeFailure("invalid JSON response") from None
        if status != expected or (expected == 200 and result.get("code") != 200):
            reason = result.get("error", {}).get("reason", "unexpected-response")
            # Never include raw response bodies, source text, or credentials.
            if reason not in {"MEMORY_UNAUTHORIZED", "MEMORY_NOT_FOUND", "MEMORY_CONFLICT", "MEMORY_SCOPE_INACTIVE", "MEMORY_INVALID_REQUEST", "MEMORY_OUTCOME_UNKNOWN", "MEMORY_UNAVAILABLE"}:
                reason = "unexpected-response"
            raise SmokeFailure(f"{method} {path}: HTTP {status} ({reason})")
        return result.get("data")

    def wait_for(predicate, step):
        deadline = time.monotonic() + args.timeout
        while time.monotonic() < deadline:
            result = predicate()
            if result:
                return result
            time.sleep(0.25)
        raise SmokeFailure(step + " remained unconfirmed")

    def activate(scope, operation):
        call("POST", "/lifecycle/activate", scope, {"operationId": operation})
        created.append(scope)

    def cleanup(scope):
        operation = "cleanup-" + scope[1]
        call("POST", "/lifecycle/deactivate", scope, {"operationId": operation})
        receipt = wait_for(lambda: (lambda op: op if op.get("physicalCleanup") else None)(call("GET", "/operations/" + operation, scope)), "lifecycle cleanup")
        cleaned.append({"userId": scope[0], "accountLifecycleId": scope[1], "receipt": receipt})

    error = None
    try:
        caps = call("GET", "/capabilities", None)
        if caps.get("dataClass") != "synthetic":
            raise SmokeFailure("target does not declare the synthetic contract")
        for i, scope in enumerate(scopes):
            activate(scope, "activate-" + str(i))
        a, b = scopes
        original = "我的朋友以前担心失业，现在已经不担心了。"
        candidate = {"operationId": "candidate-1", "source": "trustedJava", "dataClass": "synthetic", "turnId": "turn-1", "sourceMessageId": "message-1", "userText": original, "quote": original, "subject": "asQuoted", "temporal": "asQuoted", "category": "situation", "consentRef": "synthetic-consent", "policyVersion": policy}
        forged = dict(candidate, operationId="forged-1", quote="没有说过的内容")
        call("POST", "/candidates", a, forged, expected=400)
        accepted = call("POST", "/candidates", a, candidate)
        task = call("POST", "/tasks/claim", a, {"operationId": "claim-1"})
        facts = call("POST", "/tasks/publish", a, {"operationId": "publish-1", "taskId": task["taskId"], "items": [{"candidateId": accepted["candidateId"], "expectedVersion": accepted["version"], "content": original}]})
        fact = facts[0]
        wait_for(lambda: call("GET", "/facts/" + fact["factId"], a).get("syncStatus") == "confirmed", "fact sync")
        context = call("POST", "/context", a, {"query": "朋友工作", "categories": ["situation"], "deadlineMs": 2000})
        if not any(item["factId"] == fact["factId"] for item in context["facts"]):
            raise SmokeFailure("semantic retrieval did not return the published fact")
        if sum(len(item["content"]) for item in context["facts"]) > 200:
            raise SmokeFailure("context budget exceeded")
        call("GET", "/facts/" + fact["factId"], b, expected=404)
        steps.append("publish-recall-isolation")
        updated = call("PATCH", "/facts/" + fact["factId"], a, {"operationId": "edit-1", "expectedVersion": fact["version"], "content": "我的朋友现在已经找到工作了。"})
        if call("POST", "/context/validate", a, {"contextVersion": context["contextVersion"]})["valid"]:
            raise SmokeFailure("old context remained valid after correction")
        call("PATCH", "/facts/" + fact["factId"], a, {"operationId": "edit-stale", "expectedVersion": fact["version"], "content": "过期更新"}, expected=409)
        operation = call("DELETE", "/facts/" + fact["factId"], a, {"operationId": "delete-1", "expectedVersion": updated["version"]})
        if not operation["stoppedSupply"]:
            raise SmokeFailure("delete did not stop supply")
        wait_for(lambda: call("GET", "/operations/delete-1", a).get("physicalCleanup"), "fact cleanup")
        steps.append("correction-cas-delete")
        cleanup(a)
        new_lifecycle = (a[0], str(uuid.uuid4()))
        activate(new_lifecycle, "register-again")
        if call("GET", "/facts", new_lifecycle):
            raise SmokeFailure("new lifecycle read old facts")
        call("POST", "/candidates", a, candidate, expected=403)
        steps.append("re-registration-isolation")
    except (SmokeFailure, KeyError, IndexError, TypeError) as failure:
        error = str(failure) if isinstance(failure, SmokeFailure) else "unexpected response shape"
    finally:
        for scope in created:
            if any(item["accountLifecycleId"] == scope[1] for item in cleaned):
                continue
            try:
                cleanup(scope)
            except SmokeFailure:
                if error is None:
                    error = "scratch lifecycle cleanup remains unconfirmed"
    print(json.dumps({"status": "passed" if error is None else "failed", "traceId": run_id, "steps": steps, "createdScopes": [{"userId": s[0], "accountLifecycleId": s[1]} for s in created], "cleanupReceipts": cleaned, "error": error}, ensure_ascii=False, indent=2))
    return 0 if error is None else 1


if __name__ == "__main__":
    sys.exit(main())
