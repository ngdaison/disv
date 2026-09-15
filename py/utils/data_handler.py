import json
import os

DATA_FILE = 'data.json'
VIDEO_FOLDER = 'videotiktok'

def load_data():
    if not os.path.exists(DATA_FILE):
        return {}
    try:
        with open(DATA_FILE, 'r', encoding='utf-8') as f:
            return json.load(f)
    except:
        return {}

def save_data(data):
    with open(DATA_FILE, 'w', encoding='utf-8') as f:
        json.dump(data, f, indent=4, ensure_ascii=False)

def get_guild_data(data, guild_id):
    gid = str(guild_id)
    if gid not in data:
        data[gid] = {
            "settings": {},
            "users": {},
            "bad_videos": [],
            "level_roles": {},
            "auto_join_roles": {},
            "join_times": {},
            "role_menus": {},
            "as_fast": False,
            "as_dup": False
        }
    # Migration/Repair
    if "settings" not in data[gid]: data[gid]["settings"] = {}
    if "users" not in data[gid]: data[gid]["users"] = {}
    if "bad_videos" not in data[gid]: data[gid]["bad_videos"] = []
    if "level_roles" not in data[gid]: data[gid]["level_roles"] = {}
    if "auto_join_roles" not in data[gid]: data[gid]["auto_join_roles"] = {}
    if "join_times" not in data[gid]: data[gid]["join_times"] = {}
    if "role_menus" not in data[gid]: data[gid]["role_menus"] = {}
    if "as_fast" not in data[gid]:
        data[gid]["as_fast"] = any(s.get("as_fast", False) for s in data[gid]["settings"].values()) if data[gid]["settings"] else False
    if "as_dup" not in data[gid]:
        data[gid]["as_dup"] = any(s.get("as_dup", False) for s in data[gid]["settings"].values()) if data[gid]["settings"] else False
    return data[gid]

def get_channel_settings(data, guild_id, channel_id):
    g_data = get_guild_data(data, guild_id)
    cid = str(channel_id)
    guild_as_fast = g_data.get("as_fast", False)
    guild_as_dup = g_data.get("as_dup", False)
    if cid not in g_data["settings"]:
        g_data["settings"][cid] = {
            "ac_link": False, "ac_image": False, "ac_file": False, "ac_text": False,
            "as_fast": guild_as_fast, "as_dup": guild_as_dup,
            "level_enabled": False,
            "tiktok_enabled": False,
            "sticky_content": None,
            "sticky_last_id": None,
            "ai_enabled": False
        }
    # Ensure keys exist
    defaults = {
         "ac_link": False, "ac_media": False, "ac_image": False, "ac_file": False, "ac_text": False,
         "as_fast": guild_as_fast, "as_dup": guild_as_dup,
         "level_enabled": False,
         "tiktok_enabled": False,
         "sticky_content": None,
         "sticky_last_id": None,
         "ai_enabled": False
    }
    for k, v in defaults.items():
        if k not in g_data["settings"][cid]:
            g_data["settings"][cid][k] = v
            
    return g_data["settings"][cid]
