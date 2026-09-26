"""Activity summaries per employee, computed on demand from the search index.

The index already stores, for every message, the number that captured it, who sent it and when.
Joining that with the assignment history (who held a number, and for which period) answers
questions such as "what did Ana's number handle last week" without sending raw messages to the
model. Everything here honours the caller's access scope.
"""

import sqlite3
import statistics
from datetime import datetime, timedelta
from typing import Any
from zoneinfo import ZoneInfo

from lib import access
from lib.access import Window

from . import config
from .search import SearchError, _windows_sql

MAX_MESSAGES_SCANNED = 200_000
TOP_CONVERSATIONS = 10
# A customer message answered after this long is counted as answered, but not as "responsiveness".
RESPONSE_CAP_SECONDS = 24 * 3600


def parse_period(period: str, tz: ZoneInfo, now: datetime | None = None) -> tuple[datetime, datetime, str]:
    """Turn a period expression into (start, end, label). End is exclusive.

    Accepts ``today``, ``yesterday``, ``this_week``, ``last_week``, ``this_month``, ``last_month``,
    ``<N>d`` (the last N days, including today) and ``YYYY-MM-DD..YYYY-MM-DD`` (inclusive).
    """
    now = (now or datetime.now(tz)).astimezone(tz)
    today = now.replace(hour=0, minute=0, second=0, microsecond=0)
    key = period.strip().lower()

    if key == "today":
        return today, today + timedelta(days=1), "today"
    if key == "yesterday":
        return today - timedelta(days=1), today, "yesterday"
    if key in ("this_week", "last_week"):
        monday = today - timedelta(days=today.weekday())
        if key == "this_week":
            return monday, monday + timedelta(days=7), "this week (Monday to Sunday)"
        return monday - timedelta(days=7), monday, "last week (Monday to Sunday)"
    if key in ("this_month", "last_month"):
        first = today.replace(day=1)
        if key == "this_month":
            nxt = (first + timedelta(days=32)).replace(day=1)
            return first, nxt, "this month"
        prev = (first - timedelta(days=1)).replace(day=1)
        return prev, first, "last month"
    if key.endswith("d") and key[:-1].isdigit() and int(key[:-1]) > 0:
        days = int(key[:-1])
        return today - timedelta(days=days - 1), today + timedelta(days=1), f"last {days} days"
    if ".." in key:
        left, _, right = key.partition("..")
        try:
            start = datetime.fromisoformat(left).replace(tzinfo=tz)
            end = datetime.fromisoformat(right).replace(tzinfo=tz) + timedelta(days=1)
        except ValueError as exc:
            raise SearchError(f"period '{period}' is not valid: use YYYY-MM-DD..YYYY-MM-DD") from exc
        if end <= start:
            raise SearchError("period ends before it starts")
        return start, end, f"{left} to {right}"
    raise SearchError(
        f"period '{period}' is not understood. Use today, yesterday, this_week, last_week, this_month, "
        "last_month, a number of days like 7d or 30d, or YYYY-MM-DD..YYYY-MM-DD."
    )


def _clip(windows: tuple[Window, ...], start: int, end: int) -> tuple[Window, ...]:
    """Windows limited to [start, end); windows that fall outside disappear."""
    clipped = []
    for w in windows:
        lo = start if w.start is None else max(start, w.start)
        hi = end if w.end is None else min(end, w.end)
        if lo < hi:
            clipped.append(Window(w.instance_jid, lo, hi))
    return tuple(clipped)


def employee_activity_summary(
    employee_id: int,
    period: str = "7d",
    index_db_path: str = config.INDEX_DB_PATH,
    messages_db_path: str | None = None,
    display_tz: str = config.DISPLAY_TZ,
) -> dict[str, Any]:
    """What the numbers an employee operated handled in a period, while the employee operated them."""
    if messages_db_path is None:
        messages_db_path = config.resolve_messages_db_path()
    tz = ZoneInfo(display_tz)
    start, end, label = parse_period(period, tz)
    start_ts, end_ts = int(start.timestamp()), int(end.timestamp())

    employee = _load_employee(messages_db_path, employee_id)
    if employee is None:
        raise SearchError(f"No employee with id {employee_id}. Use list_employees or resolve_employee to find the id.")

    try:
        held = access.windows_for(messages_db_path, employees=[employee_id])
    except access.PolicyError as exc:
        raise SearchError(f"The organization data is not available: {exc}") from exc
    scope = access.current_scope()
    if scope.windows is not None:
        # The caller may only see the part of this person's activity that is inside its scope.
        held = tuple(w for h in held for s in scope.windows if (w := _intersect(h, s)) is not None)
    windows = _clip(held, start_ts, end_ts)

    summary: dict[str, Any] = {
        "employee": employee,
        "period": {"label": label, "from": start.isoformat(), "to": (end - timedelta(seconds=1)).isoformat()},
        "numbers": sorted({w.instance_jid for w in windows}),
    }
    if not windows:
        summary.update(_empty_totals())
        summary["note"] = "This person held no monitored number in this period, or it is outside your access."
        return summary

    try:
        conn = sqlite3.connect(f"file:{index_db_path}?mode=ro", uri=True)
    except sqlite3.OperationalError as exc:
        raise SearchError(f"The search index is not available at {index_db_path}: {exc}") from exc
    try:
        where, params = _windows_sql([windows], "m", span=False)
        rows = conn.execute(
            f"""
            SELECT m.instance_jid, m.chat_jid, m.ts, m.from_me, m.is_deleted_remote,
                   (SELECT c.chat_name FROM chunks c WHERE c.instance_jid = m.instance_jid AND c.chat_jid = m.chat_jid LIMIT 1)
            FROM messages_idx m WHERE 1 = 1{where}
            ORDER BY m.instance_jid, m.chat_jid, m.ts LIMIT ?
            """,
            (*params, MAX_MESSAGES_SCANNED + 1),
        )
        summary.update(_aggregate(rows, tz))
    finally:
        conn.close()

    labels = _instance_aliases(messages_db_path)
    summary["numbers"] = [{"instance_jid": j, **labels.get(j, {})} for j in summary["numbers"]]
    return summary


def _intersect(a: Window, b: Window) -> Window | None:
    """The overlap of two windows of the same number (None bounds are open), or None when disjoint."""
    if a.instance_jid != b.instance_jid:
        return None
    starts = [x for x in (a.start, b.start) if x is not None]
    ends = [x for x in (a.end, b.end) if x is not None]
    lo, hi = (max(starts) if starts else None), (min(ends) if ends else None)
    if lo is not None and hi is not None and lo >= hi:
        return None
    return Window(a.instance_jid, lo, hi)


def _empty_totals() -> dict[str, Any]:
    return {
        "totals": {
            "messages_received": 0,
            "messages_sent": 0,
            "conversations": 0,
            "active_days": 0,
            "deleted_by_sender": 0,
        },
        "by_day": [],
        "top_conversations": [],
        "responsiveness": {"replies_measured": 0},
    }


def _aggregate(rows: Any, tz: ZoneInfo) -> dict[str, Any]:
    received = sent = deleted = 0
    by_day: dict[str, list[int]] = {}
    chats: dict[tuple[str, str], dict[str, Any]] = {}
    response_times: list[float] = []
    waiting = 0
    scanned = 0

    prev_key: tuple[str, str] | None = None
    awaiting_since: int | None = None
    last_from_me = True

    def close_chat() -> None:
        nonlocal waiting
        if prev_key and not last_from_me and not prev_key[1].endswith("@g.us"):
            waiting += 1

    for instance_jid, chat_jid, ts, from_me, is_deleted, chat_name in rows:
        scanned += 1
        if scanned > MAX_MESSAGES_SCANNED:
            break
        key = (instance_jid, chat_jid)
        if key != prev_key:
            close_chat()
            prev_key, awaiting_since, last_from_me = key, None, True
        day = datetime.fromtimestamp(ts, tz).date().isoformat()
        counters = by_day.setdefault(day, [0, 0])
        chat = chats.setdefault(
            key,
            {
                "chat_jid": chat_jid,
                "chat_name": chat_name or chat_jid.split("@")[0],
                "received": 0,
                "sent": 0,
                "last": 0,
            },
        )
        if from_me:
            sent += 1
            counters[1] += 1
            chat["sent"] += 1
            if awaiting_since is not None and not chat_jid.endswith("@g.us"):
                delta = ts - awaiting_since
                if delta <= RESPONSE_CAP_SECONDS:
                    response_times.append(delta / 60)
                awaiting_since = None
        else:
            received += 1
            counters[0] += 1
            chat["received"] += 1
            # A message the customer took back is not waiting for an answer.
            if awaiting_since is None and not is_deleted:
                awaiting_since = ts
        if from_me or not is_deleted:
            last_from_me = bool(from_me)
        chat["last"] = max(chat["last"], ts)
        deleted += int(is_deleted or 0)
    close_chat()

    top = sorted(chats.values(), key=lambda c: c["received"] + c["sent"], reverse=True)[:TOP_CONVERSATIONS]
    responsiveness: dict[str, Any] = {"replies_measured": len(response_times)}
    if response_times:
        responsiveness["avg_first_response_minutes"] = round(statistics.fmean(response_times), 1)
        responsiveness["median_first_response_minutes"] = round(statistics.median(response_times), 1)
    responsiveness["conversations_waiting_for_reply"] = waiting

    result: dict[str, Any] = {
        "totals": {
            "messages_received": received,
            "messages_sent": sent,
            "conversations": len(chats),
            "active_days": len(by_day),
            "deleted_by_sender": deleted,
        },
        "by_day": [{"day": d, "received": v[0], "sent": v[1]} for d, v in sorted(by_day.items())],
        "top_conversations": [
            {
                "chat_jid": c["chat_jid"],
                "chat_name": c["chat_name"],
                "received": c["received"],
                "sent": c["sent"],
                "last_message": datetime.fromtimestamp(c["last"], tz).isoformat(),
            }
            for c in top
        ],
        "responsiveness": responsiveness,
    }
    if scanned > MAX_MESSAGES_SCANNED:
        result["note"] = (
            f"Only the first {MAX_MESSAGES_SCANNED} messages of the period were counted; narrow the period."
        )
    return result


def _load_employee(messages_db_path: str, employee_id: int) -> dict[str, Any] | None:
    conn = sqlite3.connect(f"file:{messages_db_path}?mode=ro", uri=True)
    try:
        row = conn.execute(
            "SELECT e.id, e.name, e.role, d.id, d.name FROM employees e LEFT JOIN departments d ON d.id = e.department_id WHERE e.id = ?",
            (employee_id,),
        ).fetchone()
    except sqlite3.OperationalError as exc:
        raise SearchError(f"The organization tables are not available: {exc}") from exc
    finally:
        conn.close()
    if row is None:
        return None
    return {
        "id": row[0],
        "name": row[1],
        "role": row[2],
        "department": {"id": row[3], "name": row[4]} if row[3] else None,
    }


def _instance_aliases(messages_db_path: str) -> dict[str, dict[str, Any]]:
    conn = sqlite3.connect(f"file:{messages_db_path}?mode=ro", uri=True)
    try:
        rows = conn.execute("SELECT phone_jid, alias FROM instances WHERE phone_jid IS NOT NULL").fetchall()
    except sqlite3.OperationalError:
        return {}
    finally:
        conn.close()
    return {jid: {"alias": alias} for jid, alias in rows if alias}
