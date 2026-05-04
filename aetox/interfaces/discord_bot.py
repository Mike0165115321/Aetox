import os
import discord
import logging
import asyncio
import time
from discord.ext import commands
from dotenv import load_dotenv

# Load environment variables
load_dotenv()
TOKEN = os.getenv("DISCORD_TOKEN")
ALLOWED_USERS = os.getenv("ALLOWED_USER_IDS", "").split(",")

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("aetox.interfaces.discord")

from aetox.core.ollama_client import OllamaClient
from aetox.core.prompt_engine import PromptEngine
from aetox.planner import AetoxPlanner
from aetox.core.dispatcher import Dispatcher
from aetox.memory.working import WorkingMemory

# Initialize Discord Bot
intents = discord.Intents.default()
intents.message_content = True
bot = commands.Bot(command_prefix="!", intents=intents)

# Persistent Instance
persistent_memory = WorkingMemory("")
shared_dispatcher = Dispatcher(persistent_memory)

class DiscordInterface:
    """
    Asynchronous Interface Layer for AetoxOS.
    Pipe mode: Discord for output, Terminal for debug/analysis.
    """
    def __init__(self, context: commands.Context):
        self.context = context

    async def stream_chat(self, stream_generator):
        """Streams AI response tokens asynchronously to Discord."""
        message = None
        full_content = ""
        last_update = 0
        update_interval = 0.5

        try:
            async for token in stream_generator:
                if token == "__NOT_CHAT__":
                    return False
                
                full_content += token
                current_time = time.time()
                
                if (current_time - last_update) > update_interval or not message:
                    if not message:
                        message = await self.context.send(full_content + " ▌")
                    else:
                        await message.edit(content=full_content + " ▌")
                    last_update = current_time
            
            if message:
                await message.edit(content=full_content)
            elif full_content:
                await self.context.send(full_content)
            return full_content
        except Exception as e:
            print(f"\n[ERROR] Streaming failed: {e}")
            await self.context.send(f"❌ **ขออภัย ระบบขัดข้อง:** {str(e)}")
            return ""

    async def send_progress(self, message: str):
        """Log progress to Terminal and Discord for visibility."""
        print(f"[PROGRESS] {message}")
        try:
            await self.context.send(f"⏳ {message}")
        except:
            pass

    async def request_approval(self, action: str, details: str) -> bool:
        """Asynchronous permission request."""
        msg = await self.context.send(
            f"⚠️ **ความปลอดภัย:** ต้องการ `{action}`\n"
            f"**รายละเอียด:** `{details}`\n"
            f"กด ✅ เพื่ออนุมัติ หรือ ❌ เพื่อปฏิเสธ"
        )
        await msg.add_reaction("✅")
        await msg.add_reaction("❌")

        def check(reaction, user):
            return user == self.context.author and str(reaction.emoji) in ["✅", "❌"] and reaction.message.id == msg.id

        try:
            reaction, _ = await bot.wait_for("reaction_add", timeout=120.0, check=check)
            return str(reaction.emoji) == "✅"
        except asyncio.TimeoutError:
            await self.context.send("⏳ **หมดเวลา:** ปฏิเสธการดำเนินการ")
            return False

@bot.event
async def on_ready():
    logger.info(f"AetoxOS Interface ready: {bot.user}")

@bot.event
async def on_message(message):
    if message.author == bot.user: return
    if message.content.startswith('!'):
        await bot.process_commands(message)
        return
    if str(message.author.id) not in ALLOWED_USERS and "*" not in ALLOWED_USERS: return
    
    ctx = await bot.get_context(message)
    await handle_task_pipe(ctx, message.content.strip())

async def handle_task_pipe(ctx, goal):
    """Main entry point - Balanced: Terminal for Debug, Discord for Interaction."""
    if not goal: return
    
    interface = DiscordInterface(ctx)
    shared_dispatcher.progress_callback = interface.send_progress
    shared_dispatcher.executor.permission_manager.approval_callback = interface.request_approval
    persistent_memory.update_context({"guild_id": ctx.guild.id if ctx.guild else None})

    async with ctx.typing():
        # 1. Internal Extraction & Analysis
        minimal_context = {"context": {}} 
        extraction = await shared_dispatcher.executor.extract_action({"description": goal}, minimal_context)
        
        est_steps = extraction.get("estimated_steps", 1)
        analysis = extraction.get("analysis", "ไม่มีบทวิเคราะห์")
        
        # --- TERMINAL LOGGING (Debug Console) ---
        print("\n" + "="*50)
        print(f"[USER GOAL] {goal}")
        print(f"[ANALYSIS] {analysis}")
        print(f"[ESTIMATED STEPS] {est_steps}")
        print("="*50 + "\n")

        # 2. Execution Lane Decision
        if est_steps > 1:
            # --- HIGH-SPEED PLANNING LANE ---
            try:
                # 1. Show the Analysis immediately as a short summary
                await ctx.send(f"🤖 **AetoxOS:** {analysis}")
                
                # Show "Preparing" status while planner works
                status_msg = await ctx.send("⏳ **กำลังคำนวณขั้นตอนและเตรียมปุ่มอนุมัติ...**")
                
                # 2. Generate the structured plan in the background
                print(f"[*] Generating structured plan for: {goal}")
                planner = AetoxPlanner()
                plan = await planner.create_plan(goal)
                
                # Remove status message
                await status_msg.delete()
                
                # Show steps and ask for approval
                plan_id = plan.get('plan_id', 'unknown')
                steps = plan.get('steps', [])
                
                plan_msg = "📝 **ขั้นตอนการทำงาน:**\n"
                for s in steps:
                    plan_msg += f"- {s.get('description')}\n"
                await ctx.send(plan_msg)

                # 3. ASK FOR APPROVAL
                is_approved = await interface.request_approval(
                    action="ดำเนินการตามแผนงาน",
                    details=f"งานที่มี {len(steps)} ขั้นตอน เพื่อบรรลุเป้าหมาย: {goal}"
                )

                if is_approved:
                    await shared_dispatcher.run_plan(plan)
                    await ctx.send("🏁 **ภารกิจเสร็จสมบูรณ์เรียบร้อยครับ!**")
                else:
                    await ctx.send("❌ **ยกเลิกแผนการทำงานแล้วครับ**")

            except Exception as e:
                print(f"[ERROR] Planning failed: {e}")
                await ctx.send(f"❌ **เกิดข้อผิดพลาดในการวางแผน:** {str(e)}")
        
        elif extraction.get("tool") == "chat":
            # --- SIMPLE CHAT LANE ---
            stream_gen = shared_dispatcher.executor.run_chat_stream(goal)
            chat_response = await interface.stream_chat(stream_gen)
            
            # ✅ บันทึกความจำ (Chat History)
            if chat_response:
                shared_dispatcher.executor.add_to_history(goal, chat_response)
        else:
            # --- SINGLE ACTION LANE ---
            try:
                result = await shared_dispatcher.executor.run_action(extraction, minimal_context)
                
                # ✅ บันทึกความจำทันที (Short-term Memory)
                shared_dispatcher.executor.add_to_history(goal, result.get("output", ""))

                if result.get("status") == "success":
                    output = result.get("output", "")
                    if output:
                        if len(output) > 1900: output = output[:1900] + "..."
                        await ctx.send(output)
                else:
                    await ctx.send(f"❌ **ล้มเหลว:** {result.get('error')}")
            except Exception as e:
                print(f"[ERROR] Execution failed: {e}")
                await ctx.send(f"❌ **ผิดพลาด:** {str(e)}")

if __name__ == "__main__":
    if TOKEN: bot.run(TOKEN)
    else: logger.error("No TOKEN found.")
