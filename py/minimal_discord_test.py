import discord
import json

with open("config.json","r",encoding="utf-8") as f:
    token = json.load(f)["bot_token"]

intents = discord.Intents.default()
intents.message_content = True
intents.members = True

client = discord.Client(intents=intents)

@client.event
async def on_ready():
    print("MINIMAL: ready", client.user)

@client.event
async def on_connect():
    print("MINIMAL: connected")

client.run(token)
