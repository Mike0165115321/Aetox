import psutil
import platform
import datetime
import time
from typing import Dict, Any

class SystemMonitorTool:
    """
    Monitor system resources like CPU, RAM, Disk, and processes.
    """
    def get_stats(self) -> str:
        """Returns a snapshot of system resource usage."""
        cpu_usage = psutil.cpu_percent(interval=1)
        memory = psutil.virtual_memory()
        disk = psutil.disk_usage('/')
        
        stats = (
            f"🖥️ **System Resource Snapshot**\n"
            f"- CPU Usage: {cpu_usage}%\n"
            f"- RAM Usage: {memory.percent}% ({round(memory.used/1024**3, 2)}GB / {round(memory.total/1024**3, 2)}GB)\n"
            f"- Disk Usage: {disk.percent}% ({round(disk.free/1024**3, 2)}GB free of {round(disk.total/1024**3, 2)}GB)\n"
        )
        return stats

    def get_top_processes(self, limit: int = 5) -> str:
        """Returns the top N processes by memory usage."""
        processes = []
        for proc in psutil.process_iter(['pid', 'name', 'memory_percent']):
            try:
                processes.append(proc.info)
            except (psutil.NoSuchProcess, psutil.AccessDenied):
                pass
        
        # Sort by memory percent
        processes = sorted(processes, key=lambda x: x['memory_percent'], reverse=True)[:limit]
        
        res = f"🔥 **Top {limit} Memory-Heavy Processes**\n"
        for p in processes:
            res += f"- {p['name']} (PID: {p['pid']}): {round(p['memory_percent'], 2)}%\n"
        return res

    def get_os_info(self) -> str:
        """Returns accurate OS information, specifically handling Windows 11."""
        os_name = platform.system()
        os_release = platform.release()
        os_version = platform.version()
        
        # Windows 11 detection (Windows 11 reports as 10, but build is >= 22000)
        if os_name == "Windows" and os_release == "10":
            build = int(os_version.split(".")[-1])
            if build >= 22000:
                os_release = "11"

        # Calculate Uptime
        uptime_seconds = time.time() - psutil.boot_time()
        uptime_hours = int(uptime_seconds // 3600)
        uptime_minutes = int((uptime_seconds % 3600) // 60)
        
        info = (
            f"ℹ️ **OS Information**\n"
            f"- OS: {os_name} {os_release} (Build {os_version})\n"
            f"- Node: {platform.node()}\n"
            f"- Uptime: {uptime_hours}h {uptime_minutes}m\n"
        )
        return info
