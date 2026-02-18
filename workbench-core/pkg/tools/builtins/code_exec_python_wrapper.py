#!/usr/bin/env python3
import io
import json
import re
import sys
import time
import types
import traceback
from contextlib import redirect_stderr, redirect_stdout

FRAME_PREFIX = "__WBX_CODE_EXEC__"
CONTROL_STDOUT = getattr(sys, "__stdout__", sys.stdout)
CONTROL_STDIN = getattr(sys, "__stdin__", sys.stdin)


class ToolError(Exception):
    def __init__(self, message, response=None):
        super().__init__(message)
        self.response = response


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
