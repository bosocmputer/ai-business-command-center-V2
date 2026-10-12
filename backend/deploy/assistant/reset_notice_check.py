"""Checks that the /new reply of this assistant carries no technical lines. Run inside the assistant (or a candidate) container:

  docker compose exec -T assistant sh -c 'PYTHONPATH=/assistant/pypatch /opt/hermes/.venv/bin/python /assistant/reset_notice_check.py'

Exit 0 and "ok" when GatewayTurnMixin._format_session_info returns an empty string after pypatch/sitecustomize.py has done its work, exit 1 otherwise
(for example a Hermes version where the class or the method has another name). upgrade-check.sh runs it.
"""
import sys

import gateway.run_turn as module

mixin = getattr(module, "GatewayTurnMixin", None)
method = getattr(mixin, "_format_session_info", None)
if method is None:
    print("not ok: GatewayTurnMixin._format_session_info is not there (a different Hermes?)")
    sys.exit(1)
if method(object()) != "":
    print("not ok: the /new reply would still carry the model lines")
    sys.exit(1)
print("ok")
