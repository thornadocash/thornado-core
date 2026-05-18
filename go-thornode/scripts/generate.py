#!/usr/bin/env python3


def mirror_simulation_modules():
    # map module to version
    require = dict()
    replace = dict()
    mode = ""
    with open("go.mod", "r") as f:
        for line in f:
            if line.startswith("require"):
                mode = "require"
            elif line.startswith("replace"):
                mode = "replace"
            elif line in {")\n", "\n"}:
                pass
            elif mode == "require":
                module, version = line.strip().split(maxsplit=1)
                require[module] = version
            elif mode == "replace":
                module, version = line.strip().split(maxsplit=1)
                replace[module] = version

    # mirror to simulation go.mod
    mode = ""
    updated = ""
    with open("test/simulation/go.mod", "r") as f:
        for line in f:
            if line.startswith("require"):
                mode = "require"
                updated += line
            elif line.startswith("replace"):
                mode = "replace"
                updated += line
            elif line in {")\n", "\n"}:
                updated += line
            elif mode == "require":
                module, version = line.strip().split(maxsplit=1)
                if module in require:
                    updated += f"\t{module} {require[module]}\n"
                else:
                    updated += line
            elif mode == "replace":
                module, version = line.strip().split(maxsplit=1)
                if module in replace:
                    updated += f"\t{module} {replace[module]}\n"
                else:
                    updated += line
            else:
                updated += line

    # write updated
    with open("test/simulation/go.mod", "w") as f:
        f.write(updated)


if __name__ == "__main__":
    mirror_simulation_modules()
