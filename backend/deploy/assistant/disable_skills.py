"""Switch off every skill that ships with Hermes, once per start.

Hermes bundles about sixty skills (coding agents, creative tools, mail, devops). The assistant has no tool that runs a skill
(see platform_toolsets), but a chat message that starts with a skill's name as a slash command, for example /claude-code, is
rewritten by the gateway into that skill's instructions unless the skill is disabled. An owner should never get a coding or
email procedure in a chat about the shop's numbers, and a procedure nobody reviewed must not sit beside the numeric rules in
SOUL.md. So every skill found in the image (and in the data volume) goes into skills.disabled in the config. A newer image
that ships new skills is covered the next time the container starts, because the list is built from what is on disk.

hermes-agent, the agent's own operating manual, is the one skill Hermes refuses to disable; it is left alone.

usage: disable_skills.py [config path] [skills directory ...]
"""
import os
import re
import sys

import yaml

ESSENTIAL = {"hermes-agent"}
DEFAULT_CONFIG = "/opt/data/config.yaml"
DEFAULT_ROOTS = ["/opt/hermes/skills", "/opt/data/skills"]
NAME_LINE = re.compile(r"^name:\s*(.+?)\s*$")


def frontmatter_name(path):
    """The 'name:' of a SKILL.md front matter block, or '' when there is none."""
    try:
        with open(path, encoding="utf-8") as handle:
            if handle.readline().strip() != "---":
                return ""
            for line in handle:
                if line.strip() == "---":
                    return ""
                match = NAME_LINE.match(line)
                if match:
                    return match.group(1).strip("'\"")
    except OSError:
        return ""
    return ""


def skill_names(roots):
    """Every skill name on disk: the directory name and the front matter name (they are normally the same)."""
    names = set()
    for root in roots:
        for directory, _, files in os.walk(root):
            if "SKILL.md" not in files:
                continue
            names.add(os.path.basename(directory))
            declared = frontmatter_name(os.path.join(directory, "SKILL.md"))
            if declared:
                names.add(declared)
    return names - ESSENTIAL


def main(argv):
    config_path = argv[1] if len(argv) > 1 else DEFAULT_CONFIG
    roots = argv[2:] or DEFAULT_ROOTS
    names = skill_names(roots)
    if not names:
        print('{"skills":"nothing to disable: no skills found"}')
        return 0
    with open(config_path, encoding="utf-8") as handle:
        config = yaml.safe_load(handle) or {}
    skills = config.get("skills")
    if not isinstance(skills, dict):
        skills = {}
    skills["disabled"] = sorted(names)
    config["skills"] = skills
    with open(config_path, "w", encoding="utf-8") as handle:
        yaml.safe_dump(config, handle, allow_unicode=True, sort_keys=False)
    print('{"skills":"disabled %d"}' % len(names))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
