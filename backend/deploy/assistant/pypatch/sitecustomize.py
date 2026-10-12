"""Loaded by Python itself at start (PYTHONPATH=/assistant/pypatch, set by entrypoint.sh for the gateway). Nothing in the image is changed.

The reply to /new ends with three technical lines (model, provider, context size) that an owner has no use for. Hermes builds them in
GatewayTurnMixin._format_session_info and has no setting to turn them off; the code that adds them skips them when the text is empty. So when the
module gateway.run_turn has been imported, this makes that one method return an empty string. If a later Hermes no longer has the class or the
method, nothing is changed and nothing breaks (the lines would simply appear again; upgrade-check runs reset_notice_check.py to notice).
"""
import importlib.abc
import importlib.util
import sys

TARGET = "gateway.run_turn"


def trim(module):
    mixin = getattr(module, "GatewayTurnMixin", None)
    if mixin is None or not hasattr(mixin, "_format_session_info"):
        return False
    mixin._format_session_info = lambda self: ""
    return True


class _Loader(importlib.abc.Loader):
    def __init__(self, inner):
        self.inner = inner

    def create_module(self, spec):
        return self.inner.create_module(spec)

    def exec_module(self, module):
        self.inner.exec_module(module)
        if trim(module):
            print('{"patch": "the /new reply carries no model lines"}', file=sys.stderr, flush=True)


class _Finder(importlib.abc.MetaPathFinder):
    def find_spec(self, name, path, target=None):
        if name != TARGET:
            return None
        sys.meta_path.remove(self)  # only once, and so the lookup below does not come back here
        spec = importlib.util.find_spec(name)
        if spec is not None and spec.loader is not None:
            spec.loader = _Loader(spec.loader)
        return spec


if TARGET not in sys.modules:
    sys.meta_path.insert(0, _Finder())
