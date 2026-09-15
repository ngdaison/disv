import asyncio
import json
import os
import shutil
from json import JSONDecodeError
from typing import Any, Dict

CONFIG_FILE = 'config_ticket.json'
TICKETS_FILE = 'tickets.json'
LOG_FILE = 'ticket_logs.json'

_storage_lock = asyncio.Lock()

DEFAULT_CONFIG = {"guilds": {}}
DEFAULT_TICKETS = {"tickets": {}}
DEFAULT_LOGS = {"logs": []}


def ensure_json_file(path: str, default: Dict[str, Any]) -> None:
    directory = os.path.dirname(path)
    if directory and not os.path.exists(directory):
        os.makedirs(directory, exist_ok=True)

    if not os.path.exists(path):
        with open(path, 'w', encoding='utf-8') as f:
            json.dump(default, f, indent=4, ensure_ascii=False)
        return

    try:
        with open(path, 'r', encoding='utf-8') as f:
            json.load(f)
    except (JSONDecodeError, ValueError):
        backup_path = f"{path}.corrupt.bak"
        shutil.copy2(path, backup_path)
        with open(path, 'w', encoding='utf-8') as f:
            json.dump(default, f, indent=4, ensure_ascii=False)


def load_json(path: str) -> Any:
    with open(path, 'r', encoding='utf-8') as f:
        return json.load(f)


async def save_json(path: str, data: Any) -> None:
    async with _storage_lock:
        temp_path = f"{path}.tmp"
        with open(temp_path, 'w', encoding='utf-8') as f:
            json.dump(data, f, indent=4, ensure_ascii=False)
            f.flush()
            os.fsync(f.fileno())

        if os.path.exists(path):
            backup_path = f"{path}.bak"
            shutil.copy2(path, backup_path)

        os.replace(temp_path, path)


async def ensure_storage_files() -> None:
    ensure_json_file(CONFIG_FILE, DEFAULT_CONFIG)
    ensure_json_file(TICKETS_FILE, DEFAULT_TICKETS)
    ensure_json_file(LOG_FILE, DEFAULT_LOGS)


async def load_config_file() -> Dict[str, Any]:
    await ensure_storage_files()
    try:
        return load_json(CONFIG_FILE)
    except (JSONDecodeError, ValueError):
        ensure_json_file(CONFIG_FILE, DEFAULT_CONFIG)
        return DEFAULT_CONFIG.copy()


async def load_tickets_file() -> Dict[str, Any]:
    await ensure_storage_files()
    try:
        return load_json(TICKETS_FILE)
    except (JSONDecodeError, ValueError):
        ensure_json_file(TICKETS_FILE, DEFAULT_TICKETS)
        return DEFAULT_TICKETS.copy()


async def load_logs_file() -> Dict[str, Any]:
    await ensure_storage_files()
    try:
        return load_json(LOG_FILE)
    except (JSONDecodeError, ValueError):
        ensure_json_file(LOG_FILE, DEFAULT_LOGS)
        return DEFAULT_LOGS.copy()
