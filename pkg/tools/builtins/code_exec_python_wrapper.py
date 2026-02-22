#!/usr/bin/env python3
import base64
import io
import json
import os
import pathlib
import re
import sys
import time
import types
import traceback
import builtins as py_builtins
from contextlib import redirect_stderr, redirect_stdout

FRAME_PREFIX = "__WBX_CODE_EXEC__"
CONTROL_STDOUT = getattr(sys, "__stdout__", sys.stdout)
CONTROL_STDIN = getattr(sys, "__stdin__", sys.stdin)

# VFS mount prefixes: paths starting with these are routed through the bridge.
_VFS_MOUNT_PREFIXES = (
    "/workspace",
    "/project",
    "/knowledge",
    "/skills",
    "/plan",
    "/memory",
    "/tasks",
    "/deliverables",
    "/log",
    "/inbox",
    "/outbox",
    "/subagents",
)


def _is_vfs_path(path):
    """Return True if path is an absolute VFS mount path."""
    if not path or not isinstance(path, str):
        return False
    p = path.strip()
    if not p.startswith("/"):
        return False
    for prefix in _VFS_MOUNT_PREFIXES:
        if p == prefix or p.startswith(prefix + "/"):
            return True
    return False


class ToolError(Exception):
    def __init__(self, message, response=None):
        super().__init__(message)
        self.response = response


DIRECT_FS_WRITE_POLICY = (
    "Direct filesystem writes are disabled in code_exec; "
    "use tools.fs_write/fs_edit/fs_append/fs_patch via tools.*."
)
DIRECT_FS_WRITE_MARKER = "policy_violation:direct_fs_write"


def _emit(frame):
    CONTROL_STDOUT.write(FRAME_PREFIX + json.dumps(frame, separators=(",", ":")) + "\n")
    CONTROL_STDOUT.flush()


def _read_frame():
    line = CONTROL_STDIN.readline()
    if not line:
        raise RuntimeError("bridge stdin closed")
    return json.loads(line)


def _jsonable(value):
    try:
        json.dumps(value)
        return value
    except Exception:
        return str(value)


class _ToolBridge:
    def __init__(self, allowed_tools):
        self.allowed = set(allowed_tools)
        self.call_count = 0
        self.next_id = 1

    def call(self, tool_name, kwargs):
        if tool_name not in self.allowed:
            raise ToolError(f"tool not allowed: {tool_name}")
        self.call_count += 1
        call_id = self.next_id
        self.next_id += 1

        _emit({
            "type": "tool_call",
            "id": call_id,
            "tool": tool_name,
            "args": kwargs,
        })

        frame = _read_frame()
        if frame.get("type") != "tool_result" or int(frame.get("id", -1)) != call_id:
            raise RuntimeError(f"invalid tool_result frame for call {call_id}")
        if not frame.get("ok", False):
            raise ToolError(frame.get("error", "tool call failed"), frame.get("response"))
        response = frame.get("response")
        if isinstance(response, dict) and response.get("ok", True) is False:
            raise ToolError(response.get("error", "tool call failed"), response)
        return response


class _ToolsProxy:
    def __init__(self, bridge):
        self._bridge = bridge

    def __getattr__(self, tool_name):
        if tool_name.startswith("_"):
            raise AttributeError(tool_name)

        def _invoke(*args, **kwargs):
            # Compatibility shape: tools.fs_read({"path": "/project"}) alongside kwargs.
            if len(args) == 1 and not kwargs and isinstance(args[0], dict):
                kwargs = args[0]
            elif len(args) != 0:
                raise ToolError(f"invalid call signature for {tool_name}: use kwargs or one dict argument")
            return self._bridge.call(tool_name, kwargs)

        return _invoke


def _direct_write_error():
    raise ToolError(f"{DIRECT_FS_WRITE_MARKER}: {DIRECT_FS_WRITE_POLICY}")


def _is_write_mode(mode):
    mode = str(mode or "r")
    return any(flag in mode for flag in ("w", "a", "x", "+"))


def _install_fs_write_policy():
    _orig_open = py_builtins.open

    def _guarded_open(file, mode="r", *args, **kwargs):
        if _is_write_mode(mode):
            _direct_write_error()
        return _orig_open(file, mode, *args, **kwargs)

    py_builtins.open = _guarded_open

    def _blocked(*args, **kwargs):
        _direct_write_error()

    # Block common os/pathlib write-side effects.
    for name in ("remove", "unlink", "rename", "replace", "rmdir", "mkdir", "makedirs"):
        if hasattr(os, name):
            setattr(os, name, _blocked)
    for name in ("write_text", "write_bytes", "touch", "mkdir", "rename", "replace", "unlink", "rmdir"):
        if hasattr(pathlib.Path, name):
            setattr(pathlib.Path, name, _blocked)


def _blocked_path_error(operation, path):
    return ToolError(
        f"code_exec file access must use VFS paths (/workspace, /project, etc.) or [path_access].allowlist; "
        f"{operation}({path!r}) is not allowed. Use tools.fs_read/fs_list with VFS paths."
    )


def _path_under_allowlist(path, allowlist):
    """Return True if path is under any allowlisted root. Paths are resolved to canonical form."""
    if not allowlist:
        return False
    try:
        resolved = os.path.abspath(os.path.normpath(path))
    except Exception:
        return False
    for root in allowlist:
        if not root:
            continue
        root_norm = os.path.normpath(root)
        if resolved == root_norm:
            return True
        sep = os.sep
        if resolved.startswith(root_norm + sep):
            return True
    return False


def _install_vfs_compat_shim(tools, path_access_allowlist=None, path_access_read_only=True, real_open=None):
    """
    Route open() and os.listdir() through the bridge when given VFS paths.
    Non-VFS paths are allowed only if under path_access_allowlist (from config).
    When read_only is True, only reads allowed on allowlisted paths; else reads and writes.
    real_open is the true builtins.open (captured before policy); used for allowlisted fall-through.
    """
    allowlist = list(path_access_allowlist) if path_access_allowlist else []
    read_only = path_access_read_only
    _orig_open = real_open if real_open is not None else py_builtins.open

    def _vfs_aware_open(file, mode="r", *args, **kwargs):
        path = str(file) if not isinstance(file, (str, bytes)) else file
        if isinstance(path, bytes):
            path = path.decode("utf-8", errors="replace")
        if _is_vfs_path(path):
            if _is_write_mode(mode):
                _direct_write_error()
            try:
                resp = tools.fs_read(path=path)
            except AttributeError:
                raise ToolError(
                    "VFS path used with open() but fs_read not available; "
                    "use tools.fs_read(path=...) or add fs_read to code_exec bridge allowlist."
                )
            if not isinstance(resp, dict):
                raise ToolError("fs_read returned unexpected type")
            if resp.get("ok", True) is False:
                raise ToolError(resp.get("error", "fs_read failed"), resp)
            text = resp.get("text", "")
            b64 = resp.get("bytesB64", "")
            if b64:
                content = base64.b64decode(b64)
                return io.BytesIO(content)
            return io.StringIO(text or "")
        if _path_under_allowlist(path, allowlist):
            if _is_write_mode(mode) and read_only:
                raise ToolError(
                    "path_access.read_only is true; writes to allowlisted paths are not allowed. "
                    "Set read_only = false in [path_access] to allow writes."
                )
            return _orig_open(file, mode, *args, **kwargs)
        raise _blocked_path_error("open", path)

    py_builtins.open = _vfs_aware_open

    _orig_listdir = os.listdir

    def _vfs_aware_listdir(path="."):
        p = str(path) if path else "."
        if _is_vfs_path(p):
            try:
                resp = tools.fs_list(path=p)
            except AttributeError:
                raise ToolError(
                    "VFS path used with os.listdir() but fs_list not available; "
                    "use tools.fs_list(path=...) or add fs_list to code_exec bridge allowlist."
                )
            if not isinstance(resp, dict):
                raise ToolError("fs_list returned unexpected type")
            if resp.get("ok", True) is False:
                raise ToolError(resp.get("error", "fs_list failed"), resp)
            entries = resp.get("entries", [])
            return list(entries) if isinstance(entries, (list, tuple)) else []
        if _path_under_allowlist(p, allowlist):
            return _orig_listdir(path)
        raise _blocked_path_error("os.listdir", p)

    os.listdir = _vfs_aware_listdir


def main():
    init = _read_frame()
    if init.get("type") != "init":
        raise RuntimeError("expected init frame")

    code = init.get("code", "")
    if not isinstance(code, str) or not code.strip():
        raise RuntimeError("code is required")

    allowed_tools = init.get("allowed_tools", [])
    if not isinstance(allowed_tools, list):
        raise RuntimeError("allowed_tools must be a list")

    bridge = _ToolBridge(allowed_tools)
    tools = _ToolsProxy(bridge)
    _real_open = py_builtins.open  # Capture before any policy replaces it
    _install_fs_write_policy()
    path_access_allowlist = init.get("path_access_allowlist") or []
    path_access_read_only = init.get("path_access_read_only", True)
    _install_vfs_compat_shim(tools, path_access_allowlist, path_access_read_only, _real_open)
    # Import-compatibility shim for model-generated code using `import tools`.
    tools_module = types.ModuleType("tools")
    tools_module.__getattr__ = lambda name: getattr(tools, name)
    sys.modules["tools"] = tools_module
    stdout_buf = io.StringIO()
    stderr_buf = io.StringIO()

    # Provide a minimal safe-ish utility surface.
    globals_dict = {
        "tools": tools,
        "ToolError": ToolError,
        "json": json,
        "re": re,
        # Compatibility aliases for model-generated JSON-style literals in Python code.
        "true": True,
        "false": False,
        "null": None,
    }

    started = time.time()
    ok = True
    error = ""
    result_value = None

    try:
        compiled = compile(code, "<code_exec>", "exec")
        with redirect_stdout(stdout_buf), redirect_stderr(stderr_buf):
            exec(compiled, globals_dict, globals_dict)
        result_value = globals_dict.get("result")
    except Exception as exc:
        ok = False
        error = str(exc)
        stderr_buf.write(traceback.format_exc())

    runtime_ms = int((time.time() - started) * 1000)
    _emit({
        "type": "final",
        "ok": ok,
        "error": error if not ok else "",
        "result": _jsonable(result_value),
        "stdout": stdout_buf.getvalue(),
        "stderr": stderr_buf.getvalue(),
        "toolCallCount": bridge.call_count,
        "runtimeMs": runtime_ms,
    })


if __name__ == "__main__":
    try:
        main()
    except Exception as exc:
        try:
            _emit({
                "type": "fatal",
                "error": str(exc),
                "traceback": traceback.format_exc(),
            })
        except Exception:
            sys.stderr.write(str(exc) + "\n")
            sys.stderr.write(traceback.format_exc() + "\n")
            sys.stderr.flush()
        raise
