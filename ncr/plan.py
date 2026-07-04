"""LLM plan step — turn a block index + context into a reading plan.

Loads prompts/00-single-shot.md, fills placeholders, calls the Anthropic API with
prompt caching on the stable system prompt, and parses the JSON reading plan. The
reconciler (not this step) is what guarantees completeness, so parsing is tolerant.
"""

from __future__ import annotations

import json
import os
import re
from pathlib import Path

PROMPTS = Path(__file__).resolve().parents[1] / "prompts"
DEFAULT_MODEL = "claude-sonnet-4-6"


def _resolve_includes(text: str) -> str:
    def repl(m: re.Match) -> str:
        inc = (PROMPTS / m.group(1).strip()).read_text()
        return inc
    return re.sub(r"\{\{include:\s*([^}]+)\}\}", repl, text)


def load_prompt(name: str = "00-single-shot.md") -> tuple[str, str]:
    """Return (system_text, user_template) parsed from a prompt markdown file."""
    raw = (PROMPTS / name).read_text()
    system = _section(raw, "System")
    user = _fenced_after(raw, "User")
    return _resolve_includes(system).strip(), user.strip()


def _section(raw: str, heading: str) -> str:
    m = re.search(rf"^##\s+{heading}\s*$(.*?)(?=^##\s|\Z)", raw, re.M | re.S)
    return m.group(1) if m else ""


def _fenced_after(raw: str, heading: str) -> str:
    body = _section(raw, heading)
    m = re.search(r"```[a-zA-Z]*\n(.*?)```", body, re.S)
    return m.group(1) if m else body


def render_user(template: str, **vars: str) -> str:
    def repl(m: re.Match) -> str:
        return str(vars.get(m.group(1).strip(), m.group(0)))
    return re.sub(r"\{\{([^}]+)\}\}", repl, template)


def extract_json(text: str) -> dict:
    """Pull the JSON object out of a model response (tolerant of fences/prose)."""
    text = text.strip()
    if text.startswith("```"):
        text = re.sub(r"^```[a-zA-Z]*\n|\n```$", "", text.strip("`\n "))
    start = text.find("{")
    if start == -1:
        raise ValueError("no JSON object in model response")
    # scan for the matching closing brace
    depth, in_str, esc = 0, False, False
    for i in range(start, len(text)):
        c = text[i]
        if in_str:
            if esc:
                esc = False
            elif c == "\\":
                esc = True
            elif c == '"':
                in_str = False
        elif c == '"':
            in_str = True
        elif c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return json.loads(text[start:i + 1])
    raise ValueError("unterminated JSON object in model response")


def make_plan(block_index: dict, files: dict, comments: list, meta: dict,
              model: str = DEFAULT_MODEL, max_tokens: int = 16000) -> dict:
    """Call the model and return the parsed reading plan (pre-reconcile)."""
    try:
        import anthropic
    except ImportError as e:
        raise RuntimeError("pip install anthropic (and set ANTHROPIC_API_KEY)") from e

    system, user_tmpl = load_prompt()
    files_txt = "\n\n".join(f"=== {p} ===\n{t}" for p, t in (files or {}).items())
    user = render_user(
        user_tmpl,
        prTitle=meta.get("title", ""),
        prNumber=meta.get("number", ""),
        prDescription=meta.get("body", ""),
        blockIndex=json.dumps(block_index, indent=0),
        files=files_txt or "(not provided)",
        comments=json.dumps(comments or [], indent=0),
    )

    client = anthropic.Anthropic(api_key=os.environ.get("ANTHROPIC_API_KEY"))
    resp = client.messages.create(
        model=model,
        max_tokens=max_tokens,
        system=[{"type": "text", "text": system,
                 "cache_control": {"type": "ephemeral"}}],
        messages=[{"role": "user", "content": user}],
    )
    text = "".join(b.text for b in resp.content if getattr(b, "type", "") == "text")
    return extract_json(text)
