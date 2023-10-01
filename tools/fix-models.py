import re
import yaml

spec = yaml.load(open("neper-types.yaml"), Loader=yaml.SafeLoader)


def addimport(uri, content):
    if uri not in content:
        lines = []
        inimport = False
        for line in content.split(b"\n"):
            if line.startswith(b"import ("):
                inimport = True

            if inimport and line.strip() == b")":
                lines.append(b'\t"%s"' % uri)
                inimport = False

            lines.append(line)
        return b"\n".join(lines)
    return content


def goupper(name):
    if name in ("id",):
        name = name.upper()
    else:
        name = name.capitalize()
    return name


def camelize(name):
    return "".join(goupper(token) for token in name.split("_"))


def gotype(prop):
    t = prop.get("type")
    ref = prop.get("$ref")
    items = prop.get("items")

    if t == "string":
        return "string"
    if t == "number":
        return "float64"
    if t == "array":
        return "[]" + gotype(items)
    if ref:
        return camelize(ref.split("/")[-1])


class CustomFix:
    def __init__(self, pattern, repl):
        self.pattern = re.compile(pattern)
        self.repl = repl

    def apply(self, content):
        return self.pattern.sub(self.repl, content)


class FixCustomTag:
    def __init__(self, prop_name, tag):
        self.prop_name = prop_name
        self.tag = tag

    def __repr__(self):
        return "FixCustomTag(%s, %s)" % (
            self.prop_name,
            self.tag,
        )

    def apply(self, content):
        placeholder = b'json:"%s,omitempty"' % self.prop_name.encode("ascii")
        newvalue = placeholder + b" " + self.tag.encode("ascii")
        print(placeholder, "->", newvalue)
        return content.replace(placeholder, newvalue)


class FixPropertyGoType:
    def __init__(self, prop_name, original_type, custom_type):
        self.prop_name = prop_name
        self.original_type = original_type
        self.custom_type = custom_type

    def __repr__(self):
        return "FixPropertyGoType(%s, %s, %s)" % (
            self.prop_name,
            self.original_type,
            self.custom_type,
        )

    def apply(self, content):
        field_name = camelize(self.prop_name)
        placeholder = b"%s %s" % (
            field_name.encode("ascii"),
            self.original_type.encode("ascii"),
        )
        custom_type = self.custom_type
        if isinstance(custom_type, dict):
            custom_type = b"%s.%s" % (
                self.custom_type["import"]["package"]
                .encode("ascii")
                .split(b"/")[-1],
                self.custom_type["type"].encode("ascii"),
            )
            content = addimport(
                self.custom_type["import"]["package"].encode("ascii"),
                content,
            )
        else:
            custom_type = custom_type.encode("ascii")
        newvalue = b"%s %s" % (field_name.encode("ascii"), custom_type)
        print(placeholder, "->", newvalue)
        return content.replace(placeholder, newvalue)


def main():
    fixes = {}

    for model_name, model in spec["definitions"].items():
        if "properties" not in model:
            continue
        for prop_name, prop in model["properties"].items():
            if "$ref" in prop and "x-go-custom-tag" in prop:
                fixes.setdefault(model_name, []).append(
                    FixCustomTag(prop_name, prop["x-go-custom-tag"])
                )
            if "x-go-type" in prop:
                prop_type = gotype(prop)
                fixes.setdefault(model_name, []).append(
                    FixPropertyGoType(prop_name, prop_type, prop["x-go-type"])
                )
                # The actual array items may be pointers, and this case is not
                # handled property by "gotype()". The following is a dirty hack
                # to workaround this
                if prop_type.startswith("[]"):
                    prop_type = "[]*" + prop_type[2:]
                fixes.setdefault(model_name, []).append(
                    FixPropertyGoType(prop_name, prop_type, prop["x-go-type"])
                )

    for model, fixes in fixes.items():
        print("Applying fixes on", model)
        with open("models/%s.go" % model, "rb") as f:
            print("models/%s.go" % model)
            content = f.read()
        for fix in fixes:
            print("  *", fix)
            content = fix.apply(content)
        with open("models/%s.go" % model, "wb") as f:
            f.write(content)


if __name__ == "__main__":
    main()
