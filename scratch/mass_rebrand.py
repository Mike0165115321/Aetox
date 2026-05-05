import os
import re

def mass_replace(root_dir, replacements):
    for root, dirs, files in os.walk(root_dir):
        # Skip some directories
        if any(skip in root for skip in [".git", "__pycache__", ".venv", ".antigravity", "scratch"]):
            continue
            
        for file in files:
            # Skip binary or irrelevant files
            if not file.endswith((".py", ".md", ".yaml", ".bat", ".txt", ".sh")):
                continue
                
            file_path = os.path.join(root, file)
            try:
                with open(file_path, "r", encoding="utf-8") as f:
                    content = f.read()
                
                new_content = content
                for old, new in replacements.items():
                    # Case-sensitive replacement for AetoxOS -> AetoxClaw
                    new_content = re.sub(old, new, new_content)
                
                if new_content != content:
                    print(f"Updating: {file_path}")
                    with open(file_path, "w", encoding="utf-8") as f:
                        f.write(new_content)
            except Exception as e:
                print(f"Error processing {file_path}: {e}")

replacements = {
    r"AetoxOS": "AetoxClaw",
    r"aetoxos": "aetoxclaw",
    r"AETOXOS": "AETOXCLAW",
    r"14B": "8B",
    r"14b": "8b",
    r"qwen2\.5:14b": "qwen3:8b",
    r"Qwen 2\.5 14B": "Qwen 3 8B"
}

if __name__ == "__main__":
    mass_replace(".", replacements)
    
    # Rename Aetoxos.md if it exists
    if os.path.exists("Aetoxos.md"):
        print("Renaming Aetoxos.md -> AetoxClaw.md")
        os.rename("Aetoxos.md", "AetoxClaw.md")
