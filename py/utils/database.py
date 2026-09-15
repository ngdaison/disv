import aiosqlite
import json
import os

DB_FILE = "database.db"

class Database:
    def __init__(self):
        self.db_name = DB_FILE

    async def initialize(self):
        async with aiosqlite.connect(self.db_name) as db:
            # Guild Configs
            await db.execute("""
                CREATE TABLE IF NOT EXISTS guild_configs (
                    guild_id INTEGER PRIMARY KEY,
                    
                    -- Anti Content
                    ac_enabled BOOLEAN DEFAULT 0,
                    ac_block_links BOOLEAN DEFAULT 1,
                    ac_block_images BOOLEAN DEFAULT 0,
                    ac_block_files BOOLEAN DEFAULT 0,
                    
                    -- Anti Spam
                    as_enabled BOOLEAN DEFAULT 1,
                    as_fast_msg_limit INTEGER DEFAULT 2, -- "1s link 2 tin" -> interpreted as limit 2 msg in 1s for consistency? or strict 2 msg/1s
                    as_fast_time_limit INTEGER DEFAULT 1,
                    as_dup_msg_limit INTEGER DEFAULT 2,
                    as_dup_time_limit INTEGER DEFAULT 60,

                    -- TikTok
                    tiktok_enabled BOOLEAN DEFAULT 1,

                    -- Leveling
                    leveling_enabled BOOLEAN DEFAULT 1,
                    xp_difficulty REAL DEFAULT 1.0, -- Multiplier for XP requirement
                    level_up_channel_id INTEGER, -- Specific channel for announcements (optional)
                    
                    -- Utility
                    sticky_enabled BOOLEAN DEFAULT 0,
                    sticky_channel_id INTEGER,
                    sticky_content TEXT,
                    sticky_last_message_id INTEGER,
                    
                    mute_role_id INTEGER
                )
            """)

            # Allowed XP Channels (Many-to-Many logic handled by simple JSON or separate table. Using JSON for simplicity here)
            # Actually, let's use a separate table for cleaner relational design
            await db.execute("""
                CREATE TABLE IF NOT EXISTS xp_allowed_channels (
                    guild_id INTEGER,
                    channel_id INTEGER,
                    PRIMARY KEY (guild_id, channel_id)
                )
            """)
            
            # Users Leveling Data
            await db.execute("""
                CREATE TABLE IF NOT EXISTS users (
                    guild_id INTEGER,
                    user_id INTEGER,
                    xp INTEGER DEFAULT 0,
                    level INTEGER DEFAULT 1, -- Start at level 1 as requested
                    PRIMARY KEY (guild_id, user_id)
                )
            """)

            # Level Rewards
            await db.execute("""
                CREATE TABLE IF NOT EXISTS level_rewards (
                    guild_id INTEGER,
                    level INTEGER,
                    role_id INTEGER,
                    PRIMARY KEY (guild_id, level, role_id)
                )
            """)
            
            # Offense Tracking (Persistent for 48h check even if bot restarts)
            await db.execute("""
                CREATE TABLE IF NOT EXISTS offenses (
                    guild_id INTEGER,
                    user_id INTEGER,
                    type TEXT, -- 'content' or 'spam'
                    count INTEGER DEFAULT 0,
                    last_offense_time REAL,
                    first_offense_time_window REAL,
                    PRIMARY KEY (guild_id, user_id, type)
                )
            """)

            await db.commit()

    async def get_guild_config(self, guild_id: int):
        async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            cursor = await db.execute("SELECT * FROM guild_configs WHERE guild_id = ?", (guild_id,))
            row = await cursor.fetchone()
            
            if row:
                return dict(row)
            else:
                await db.execute("INSERT INTO guild_configs (guild_id) VALUES (?)", (guild_id,))
                await db.commit()
                cursor = await db.execute("SELECT * FROM guild_configs WHERE guild_id = ?", (guild_id,))
                row = await cursor.fetchone()
                return dict(row)

    async def update_guild_config(self, guild_id: int, **kwargs):
        if not kwargs: return
        columns = ", ".join(f"{key} = ?" for key in kwargs.keys())
        values = list(kwargs.values())
        values.append(guild_id)
        async with aiosqlite.connect(self.db_name) as db:
            await db.execute(f"UPDATE guild_configs SET {columns} WHERE guild_id = ?", values)
            await db.commit()

    # --- XP Channel Management ---
    async def get_xp_allowed_channels(self, guild_id: int):
        async with aiosqlite.connect(self.db_name) as db:
            cursor = await db.execute("SELECT channel_id FROM xp_allowed_channels WHERE guild_id = ?", (guild_id,))
            rows = await cursor.fetchall()
            return [row[0] for row in rows]

    async def set_xp_allowed_channel(self, guild_id: int, channel_id: int, allowed: bool):
        async with aiosqlite.connect(self.db_name) as db:
            if allowed:
                await db.execute("INSERT OR IGNORE INTO xp_allowed_channels (guild_id, channel_id) VALUES (?, ?)", (guild_id, channel_id))
            else:
                await db.execute("DELETE FROM xp_allowed_channels WHERE guild_id = ? AND channel_id = ?", (guild_id, channel_id))
            await db.commit()

    # --- Leveling Methods ---
    async def get_user_xp(self, guild_id: int, user_id: int):
        async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            cursor = await db.execute("SELECT xp, level FROM users WHERE guild_id = ? AND user_id = ?", (guild_id, user_id))
            row = await cursor.fetchone()
            if row: return dict(row)
            else: return {"xp": 0, "level": 1}

    async def update_user_xp(self, guild_id: int, user_id: int, xp: int, level: int):
        async with aiosqlite.connect(self.db_name) as db:
            await db.execute("""
                INSERT INTO users (guild_id, user_id, xp, level) 
                VALUES (?, ?, ?, ?)
                ON CONFLICT(guild_id, user_id) DO UPDATE SET xp = ?, level = ?
            """, (guild_id, user_id, xp, level, xp, level))
            await db.commit()

    async def get_level_rewards(self, guild_id: int):
         async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            cursor = await db.execute("SELECT level, role_id FROM level_rewards WHERE guild_id = ? ORDER BY level ASC", (guild_id,))
            rows = await cursor.fetchall()
            return [dict(row) for row in rows]
    
    async def add_level_reward(self, guild_id: int, level: int, role_id: int):
        async with aiosqlite.connect(self.db_name) as db:
             await db.execute("INSERT OR REPLACE INTO level_rewards (guild_id, level, role_id) VALUES (?, ?, ?)", (guild_id, level, role_id))
             await db.commit()

    # --- Offense Tracking (Persistent) ---
    async def get_offense(self, guild_id: int, user_id: int, type: str):
         async with aiosqlite.connect(self.db_name) as db:
            db.row_factory = aiosqlite.Row
            cursor = await db.execute("SELECT * FROM offenses WHERE guild_id = ? AND user_id = ? AND type = ?", (guild_id, user_id, type))
            row = await cursor.fetchone()
            if row: return dict(row)
            return None

    async def update_offense(self, guild_id: int, user_id: int, type: str, count: int, last_time: float, first_window: float):
         async with aiosqlite.connect(self.db_name) as db:
            await db.execute("""
                INSERT INTO offenses (guild_id, user_id, type, count, last_offense_time, first_offense_time_window)
                VALUES (?, ?, ?, ?, ?, ?)
                ON CONFLICT(guild_id, user_id, type) DO UPDATE SET count = ?, last_offense_time = ?, first_offense_time_window = ?
            """, (guild_id, user_id, type, count, last_time, first_window, count, last_time, first_window))
            await db.commit()
            
    async def clear_offense(self, guild_id: int, user_id: int, type: str):
        async with aiosqlite.connect(self.db_name) as db:
            await db.execute("DELETE FROM offenses WHERE guild_id = ? AND user_id = ? AND type = ?", (guild_id, user_id, type))
            await db.commit()

db = Database()
