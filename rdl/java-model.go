// Copyright 2015 Yahoo Inc.
// Licensed under the terms of the Apache version 2.0 license. See LICENSE file for terms.

package main

import (
	"bufio"
	"fmt"
	"github.com/ardielle/ardielle-go/rdl"
	"strings"
)

type javaModelGenerator struct {
	registry rdl.TypeRegistry
	schema   *rdl.Schema
	name     string
	writer   *bufio.Writer
	err      error
	ns       string
	jackson  bool
}

// GenerateJavaModel generates the model code for the types defined in the RDL schema.
func GenerateJavaModel(banner string, schema *rdl.Schema, outdir string, ns string) error {
	packageDir, err := javaGenerationDir(outdir, schema, ns)
	if err != nil {
		return err
	}
	registry := rdl.NewTypeRegistry(schema)
	for _, t := range schema.Types {
		tName, _, _ := rdl.TypeInfo(t)
		if strings.HasPrefix(string(tName), "rdl.") {
			continue
		}
		err := generateJavaType(banner, schema, registry, packageDir, t, ns)
		if err != nil {
			return err
		}
	}
	cName := capitalize(string(schema.Name)) + "Schema"
	out, file, _, err := outputWriter(packageDir, cName, ".java")
	if err != nil {
		return err
	}
	err = javaGenerateSchema(schema, cName, out, ns, banner)
	out.Flush()
	file.Close()
	if err != nil {
		return err
	}
	return nil
}

func generateJavaType(banner string, schema *rdl.Schema, registry rdl.TypeRegistry, outdir string, t *rdl.Type, ns string) error {
	tName, _, _ := rdl.TypeInfo(t)
	bt := registry.BaseType(t)
	switch bt {
	case rdl.BaseTypeStruct:
	case rdl.BaseTypeUnion:
	case rdl.BaseTypeEnum:
	default:
		return nil
	}
	cName := capitalize(string(tName))
	out, file, _, err := outputWriter(outdir, cName, ".java")
	if err != nil {
		return err
	}
	if file != nil {
		defer file.Close()
	}
	gen := &javaModelGenerator{registry, schema, string(tName), out, nil, ns, true}
	gen.emitHeader(banner, ns, bt, t)
	switch bt {
	case rdl.BaseTypeStruct:
		gen.emit("\n")
		gen.emitStruct(t, cName)
	case rdl.BaseTypeUnion:
		gen.emit("\n")
		gen.emitUnion(t)
	case rdl.BaseTypeArray:
		gen.emit("\n")
		gen.emitArray(t)
	case rdl.BaseTypeEnum:
		gen.emit("\n")
		gen.emitTypeComment(t)
		gen.emitEnum(t)
	}
	out.Flush()
	return gen.err
}

func (gen *javaModelGenerator) emit(s string) {
	if gen.err == nil {
		_, err := gen.writer.WriteString(s)
		if err != nil {
			gen.err = err
		}
	}
}

func (gen *javaModelGenerator) isFieldPrimitiveType(f *rdl.StructFieldDef) bool {
	switch gen.registry.FindBaseType(f.Type) {
	case rdl.BaseTypeBool, rdl.BaseTypeInt8, rdl.BaseTypeInt16, rdl.BaseTypeInt32, rdl.BaseTypeInt64, rdl.BaseTypeFloat32, rdl.BaseTypeFloat64:
		return !f.Optional
	default:
		return false
	}
}

func (gen *javaModelGenerator) structHasFieldDefault(t *rdl.StructTypeDef) bool {
	if t != nil {
		for _, f := range t.Fields {
			if f.Default != nil {
				switch gen.registry.FindBaseType(f.Type) {
				case rdl.BaseTypeString, rdl.BaseTypeSymbol, rdl.BaseTypeUUID, rdl.BaseTypeTimestamp:
					if f.Default.(string) != "" {
						return true
					}
				case rdl.BaseTypeInt8, rdl.BaseTypeInt16, rdl.BaseTypeInt32, rdl.BaseTypeInt64, rdl.BaseTypeFloat32, rdl.BaseTypeFloat64:
					if f.Default.(float64) != 0 {
						return true
					}
				case rdl.BaseTypeBool:
					if f.Default.(bool) {
						return true
					}
				}
			}
		}
	}
	return false
}

func (gen *javaModelGenerator) addIndirectImports(t *rdl.Type, types map[string]int) {
	switch t.Variant {
	case rdl.TypeVariantStructTypeDef:
		fields := flattenedFields(gen.registry, t)
		for _, f := range fields {
			if f.Type == "Map" {
				types["java.util.Map"] = 1
			} else if f.Type == "Array" {
				types["java.util.List"] = 1
			}
		}
	}
}

func (gen *javaModelGenerator) indirectImports(t *rdl.Type) string {
	s := ""
	types := make(map[string]int)
	gen.addIndirectImports(t, types)
	for k, _ := range types {
		s += "import " + k + ";\n"
	}
	return s
}
func (gen *javaModelGenerator) emitHeader(banner string, ns string, bt rdl.BaseType, t *rdl.Type) {
	gen.emit(javaGenerationHeader(banner))
	gen.emit("\n\n")
	pack := javaGenerationPackage(gen.schema, gen.ns)
	if pack != "" {
		gen.emit("package " + javaGenerationPackage(gen.schema, gen.ns) + ";\n")
	}
	simports := gen.indirectImports(t)
	if simports != "" {
		gen.emit(simports)
	}
	if ns != "com.yahoo.rdl" {
		gen.emit("import com.yahoo.rdl.*;\n")
	}
	if gen.jackson {
		if bt == rdl.BaseTypeUnion {
			gen.emit("import java.io.IOException;\n")
			gen.emit("import com.fasterxml.jackson.databind.JsonDeserializer;\n")
			gen.emit("import com.fasterxml.jackson.databind.annotation.JsonDeserialize;\n")
			gen.emit("import com.fasterxml.jackson.core.JsonParser;\n")
			gen.emit("import com.fasterxml.jackson.core.JsonToken;\n")
			gen.emit("import com.fasterxml.jackson.databind.DeserializationContext;\n")
			gen.emit("import com.fasterxml.jackson.core.JsonProcessingException;\n")
			gen.emit("import com.fasterxml.jackson.databind.ObjectMapper;\n")
			gen.emit("import com.fasterxml.jackson.databind.node.ObjectNode;\n")
		}
		if bt != rdl.BaseTypeEnum {
			gen.emit("import com.fasterxml.jackson.databind.annotation.JsonSerialize;\n")
		}
	}
}

func (gen *javaModelGenerator) emitTypeComment(t *rdl.Type) {
	tName, _, tComment := rdl.TypeInfo(t)
	s := string(tName) + " -"
	if tComment != "" {
		s += " " + tComment
	}
	gen.emit(formatComment(s, 0, 80))
}

func javaType(reg rdl.TypeRegistry, rdlType rdl.TypeRef, optional bool, items rdl.TypeRef, keys rdl.TypeRef) string {
	t := reg.FindType(rdlType)
	if t == nil || t.Variant == 0 {
		panic("Cannot find type '" + rdlType + "'")
	}
	bt := reg.BaseType(t)
	switch bt {
	case rdl.BaseTypeAny:
		return "Object"
	case rdl.BaseTypeString:
		return "String"
	case rdl.BaseTypeSymbol, rdl.BaseTypeTimestamp, rdl.BaseTypeUUID:
		return string(rdlType)
	case rdl.BaseTypeBool:
		if optional {
			return "Boolean"
		}
		return "boolean"
	case rdl.BaseTypeInt8:
		if optional {
			return "Byte"
		}
		return "byte"
	case rdl.BaseTypeInt16:
		if optional {
			return "Short"
		}
		return "short"
	case rdl.BaseTypeInt32:
		if optional {
			return "Integer"
		}
		return "int"
	case rdl.BaseTypeInt64:
		if optional {
			return "Long"
		}
		return "long"
	case rdl.BaseTypeFloat32:
		if optional {
			return "Float"
		}
		return "float"
	case rdl.BaseTypeFloat64:
		if optional {
			return "Double"
		}
		return "double"
	case rdl.BaseTypeArray:
		i := rdl.TypeRef("Any")
		switch t.Variant {
		case rdl.TypeVariantArrayTypeDef:
			i = t.ArrayTypeDef.Items
		default:
			if items != "" && items != "Any" {
				i = items
			}
		}
		gitems := javaType(reg, rdl.TypeRef(i), false, "", "")
		//return gitems + "[]" //if arrays, not lists
		return "List<" + gitems + ">"
	case rdl.BaseTypeMap:
		k := rdl.TypeRef("Any")
		i := rdl.TypeRef("Any")
		switch t.Variant {
		case rdl.TypeVariantMapTypeDef:
			k = t.MapTypeDef.Keys
			i = t.MapTypeDef.Items
		default:
			if keys != "" && keys != "Any" {
				k = keys
			}
			if items != "" && items != "Any" {
				i = items
			}
		}
		gkeys := javaType(reg, k, false, "", "")
		gitems := javaType(reg, i, false, "", "")
		return "Map<" + gkeys + ", " + gitems + ">"
	case rdl.BaseTypeStruct:
		if strings.HasPrefix(string(rdlType), "rdl.") {
			return string(rdlType)[4:]
		}
		switch t.Variant {
		case rdl.TypeVariantStructTypeDef:
			if t.StructTypeDef.Name == "Struct" {
				return "Object"
			}
		}
		return string(rdlType)
	default:
		return string(rdlType)
	}
}

func (gen *javaModelGenerator) emitUnion(t *rdl.Type) {
	if gen.err == nil {
		switch t.Variant {
		case rdl.TypeVariantUnionTypeDef:
			gen.emitTypeComment(t)
			ut := t.UnionTypeDef
			tName := ut.Name
			uName := capitalize(string(tName))
			if gen.jackson {
				gen.emit("@JsonSerialize(include = JsonSerialize.Inclusion.NON_NULL)\n")
				gen.emit(fmt.Sprintf("@JsonDeserialize(using = %s.%sJsonDeserializer.class)\n", uName, uName))
			}
			gen.emit(fmt.Sprintf("public final class %s {\n", uName))
			gen.emit(fmt.Sprintf("    public enum %sVariant {\n", uName))
			for i, vtype := range ut.Variants {
				if i == 0 {
					gen.emit("        ")
				} else {
					gen.emit(",\n        ")
				}
				gen.emit(fmt.Sprintf("%s", vtype))
			}
			gen.emit("\n    }\n\n")
			gen.emit("    @com.fasterxml.jackson.annotation.JsonIgnore\n")
			gen.emit(fmt.Sprintf("    public %sVariant variant;\n\n", uName))
			for _, v := range ut.Variants {
				vtype := javaType(gen.registry, v, true, "", "")
				gen.emit(fmt.Sprintf("    @RdlOptional public %s %s;\n", vtype, v))
			}

			gen.emit("    @Override\n    public boolean equals(Object another) {\n")
			gen.emit("        if (this != another) {\n")
			gen.emit(fmt.Sprintf("            if (another == null || another.getClass() != %s.class) {\n", uName))
			gen.emit("                return false;\n")
			gen.emit("            }\n")
			gen.emit(fmt.Sprintf("            %s a = (%s) another;\n", uName, uName))
			gen.emit("            if (variant == a.variant) {\n")
			gen.emit("                switch (variant) {\n")
			for _, fname := range ut.Variants {
				gen.emit(fmt.Sprintf("                case %s:\n", fname))
				gen.emit(fmt.Sprintf("                    return %s.equals(a.%s);\n", fname, fname))
			}
			gen.emit("                }\n")
			gen.emit("            }\n")
			gen.emit("        }\n")
			gen.emit("        return false;\n")
			gen.emit("    }\n\n")

			gen.emit(fmt.Sprintf("\n    public static class %sJsonDeserializer extends JsonDeserializer<%s> {\n", uName, uName))
			gen.emit("        @Override\n")
			gen.emit(fmt.Sprintf("        public %s deserialize(JsonParser jp, DeserializationContext ctxt) throws IOException, JsonProcessingException {\n", uName))
			gen.emit("            JsonToken tok = jp.nextToken();\n")
			gen.emit("            if (tok != JsonToken.FIELD_NAME) {\n")
			gen.emit(fmt.Sprintf("                throw new IOException(\"Cannot deserialize %s - no valid variant present\");\n", uName))
			gen.emit("            }\n")
			gen.emit("            String svariant = jp.getCurrentName();\n")
			gen.emit("            tok = jp.nextToken();\n")
			gen.emit(fmt.Sprintf("            %s t = null;\n", uName))

			var boolVariants []rdl.TypeRef
			var numberVariants []rdl.TypeRef
			var stringVariants []rdl.TypeRef
			var arrayVariants []rdl.TypeRef
			var objectVariants []rdl.TypeRef

			mapVariants := make(map[string]rdl.BaseType)
			for _, vtype := range ut.Variants {
				t := gen.registry.FindType(vtype)
				if t == nil || t.Variant == 0 {
					gen.err = fmt.Errorf("Cannot find type '%v'", vtype)
					return
				}
				bt := gen.registry.BaseType(t)
				mapVariants[string(vtype)] = bt
				switch bt {
				case rdl.BaseTypeString, rdl.BaseTypeSymbol, rdl.BaseTypeTimestamp, rdl.BaseTypeUUID, rdl.BaseTypeEnum:
					stringVariants = append(stringVariants, vtype)
				case rdl.BaseTypeBool:
					boolVariants = append(boolVariants, vtype)
				case rdl.BaseTypeInt8, rdl.BaseTypeInt16, rdl.BaseTypeInt32, rdl.BaseTypeInt64, rdl.BaseTypeFloat32, rdl.BaseTypeFloat64:
					numberVariants = append(numberVariants, vtype)
				case rdl.BaseTypeArray:
					arrayVariants = append(arrayVariants, vtype)
				case rdl.BaseTypeMap, rdl.BaseTypeStruct:
					objectVariants = append(objectVariants, vtype)
				}
			}
			if numberVariants != nil {
				gen.emit("            if (tok == JsonToken.VALUE_NUMBER_INT || tok == JsonToken.VALUE_NUMBER_FLOAT) {\n")
				gen.emit("                switch (svariant) {\n")
				for _, v := range numberVariants {
					vtype := javaType(gen.registry, v, true, "", "")
					gen.emit(fmt.Sprintf("                case %q:\n", v))
					s := vtype
					if s == "Integer" {
						s = "Int"
					}
					gen.emit(fmt.Sprintf("                    t = new %s(jp.get%sValue());\n", uName, s))
					gen.emit("                    break;\n")
				}
				gen.emit("               default:\n")
				gen.emit(fmt.Sprintf("                    throw new IOException(\"Cannot deserialize %s - bad type variant: \" + svariant);\n", uName))
				gen.emit("                }\n")
				gen.emit("                tok = jp.nextToken();\n")
				gen.emit("                return t;\n")
				gen.emit("            }\n")
			}
			if stringVariants != nil {
				gen.emit("            if (tok == JsonToken.VALUE_STRING) {\n")
				gen.emit("                switch (svariant) {\n")
				for _, v := range stringVariants {
					gen.emit(fmt.Sprintf("                case %q:\n", v))
					vtype := javaType(gen.registry, v, true, "", "")
					if vtype == "String" {
						gen.emit(fmt.Sprintf("                    t = new %s(jp.getText());\n", uName))
					} else {
						gen.emit(fmt.Sprintf("                    t = new %s(%s.%s.fromString(jp.getText()));\n", uName, gen.ns, v))
					}
					gen.emit("                    break;\n")
				}
				gen.emit("                default:\n")
				gen.emit(fmt.Sprintf("                    throw new IOException(\"Cannot deserialize %s - bad type variant: \" + svariant);\n", uName))
				gen.emit("                }\n")
				gen.emit("                tok = jp.nextToken();\n")
				gen.emit("                return t;\n")
				gen.emit("            }\n")
			}
			if boolVariants != nil {
				gen.emit("            if (tok == JsonToken.VALUE_TRUE || tok == JsonToken.VALUE_FALSE) {\n")
				gen.emit("                switch (svariant) {\n")
				for _, v := range boolVariants {
					gen.emit(fmt.Sprintf("                case %q:\n", v))
					gen.emit(fmt.Sprintf("                    t = new %s(jp.getBooleanValue());\n", uName))
					gen.emit("                    break;\n")
				}
				gen.emit("                default:\n")
				gen.emit(fmt.Sprintf("                    throw new IOException(\"Cannot deserialize %s - bad type variant: \" + svariant);\n", uName))
				gen.emit("                }\n")
				gen.emit("                tok = jp.nextToken();\n")
				gen.emit("                return t;\n")
				gen.emit("            }\n")
			}
			if arrayVariants != nil {
				//gen.emit("            if tok == JsonToken.START_ARRAY {
				panic("NYI - union of arrays")
			}
			if objectVariants != nil {
				gen.emit("            if (tok == JsonToken.START_OBJECT) {\n")
				gen.emit("                switch (svariant) {\n")
				for _, v := range objectVariants {
					vtype := javaType(gen.registry, v, true, "", "")
					gen.emit(fmt.Sprintf("                case %q:\n", vtype))
					gen.emit(fmt.Sprintf("                    t = new %s(jp.readValueAs(%s.class));\n", uName, vtype))
					gen.emit("                    break;\n")
				}
				gen.emit("                default:\n")
				gen.emit(fmt.Sprintf("                    throw new IOException(\"Cannot deserialize %s - bad type variant: \" + svariant);\n", uName))
				gen.emit("                }\n")
				gen.emit("                if (t != null) {\n")
				gen.emit("                    tok = jp.nextToken();\n")
				gen.emit("                    if (tok == JsonToken.END_OBJECT) {\n")
				gen.emit("                        return t;\n")
				gen.emit("                    }\n")
				gen.emit(fmt.Sprintf("                    throw new IOException(\"Cannot deserialize %s - more than one variant present\");\n", uName))
				gen.emit("                }\n")
				gen.emit("            }\n")
			}
			gen.emit(fmt.Sprintf("            throw new IOException(\"Cannot deserialize %s - no variant present\");\n", uName))
			gen.emit("        }\n")
			gen.emit("    }\n")

			gen.emit(fmt.Sprintf("\n    public %s() {\n    }\n", uName))
			for _, v := range ut.Variants {
				vtype := javaType(gen.registry, v, true, "", "")
				vname := uncapitalize(string(v))
				gen.emit(fmt.Sprintf("\n    public %s(%s %s) {\n", uName, vtype, vname))
				gen.emit(fmt.Sprintf("        this.variant = %sVariant.%s;\n", uName, v))
				gen.emit(fmt.Sprintf("        this.%s = %s;\n", v, vname))
				gen.emit("    }\n")
			}
			if false {
				gen.emit("\n    public String toString() {\n")
				gen.emit("        switch (variant) {\n")
				for _, v := range ut.Variants {
					vname := uncapitalize(string(v))
					gen.emit(fmt.Sprintf("        case %s:\n            return \"<\" + %s.toString() + \">\";\n", v, vname))
				}
				gen.emit("        }\n")
				gen.emit("        return super.toString();\n")
				gen.emit("    }\n")
			}
			gen.emit("}\n")
		default:
			gen.err = fmt.Errorf("Bad union definition: %v", t)
		}
	}
}

func (gen *javaModelGenerator) literal(lit interface{}) string {
	switch v := lit.(type) {
	case string:
		return fmt.Sprintf("%q", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case int16:
		return fmt.Sprintf("%d", v)
	case int8:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case float64:
		return fmt.Sprintf("%g", v)
	case float32:
		return fmt.Sprintf("%g", v)
	default: //bool, enum
		return fmt.Sprintf("%v", lit)
	}
}

func (gen *javaModelGenerator) emitArray(t *rdl.Type) {
	if gen.err == nil {
		switch t.Variant {
		case rdl.TypeVariantArrayTypeDef:
			at := t.ArrayTypeDef
			gen.emitTypeComment(t)
			ftype := javaType(gen.registry, at.Type, false, at.Items, "")
			gen.emit(fmt.Sprintf("type %s %s\n\n", at.Name, ftype))
		default:
			tName, tType, _ := rdl.TypeInfo(t)
			gtype := javaType(gen.registry, tType, false, "", "")
			gen.emitTypeComment(t)
			gen.emit(fmt.Sprintf("type %s %s\n\n", tName, gtype))
		}
	}
}

func (gen *javaModelGenerator) emitStruct(t *rdl.Type, cName string) {
	if gen.err == nil {
		switch t.Variant {
		case rdl.TypeVariantStructTypeDef:
			st := t.StructTypeDef
			f := flattenedFields(gen.registry, t)
			gen.emitTypeComment(t)
			gen.emitStructFields(f, st.Name, st.Comment, cName, st.Closed)
			if gen.structHasFieldDefault(st) {
				gen.emit("\n    //\n    // sets up the instance according to its default field values, if any\n    //\n")
				gen.emit(fmt.Sprintf("    public %s init() {\n", st.Name))
				for _, f := range f {
					if f.Default != nil {
						gen.emit(fmt.Sprintf("        if (%s == null) {\n", f.Name))
						gen.emit(fmt.Sprintf("            %s = %s;\n", f.Name, gen.literal(f.Default)))
						gen.emit("        }\n")
					}
				}
				gen.emit("        return this;\n")
				gen.emit("    }\n")
			}
			gen.emit("}\n")
		case rdl.TypeVariantAliasTypeDef:
			gen.emitTypeComment(t)
			at := t.AliasTypeDef
			var fields []*rdl.StructFieldDef
			gen.emitStructFields(fields, at.Name, at.Comment, cName, false)
			gen.emit("}\n")
		default:
			panic(fmt.Sprintf("Unreasonable struct typedef: %v", t.Variant))
		}
	}
}

func (gen *javaModelGenerator) emitEnum(t *rdl.Type) {
	if gen.err != nil {
		return
	}
	et := t.EnumTypeDef
	name := capitalize(string(et.Name))
	gen.emit(fmt.Sprintf("public enum %s {", name))
	for i, elem := range et.Elements {
		sym := elem.Symbol
		if i > 0 {
			gen.emit(",\n")
		} else {
			gen.emit("\n")
		}
		gen.emit(fmt.Sprintf("    %s", sym))
	}
	gen.emit(";\n")
	gen.emit(fmt.Sprintf("\n    public static %s fromString(String v) {\n", name))
	gen.emit(fmt.Sprintf("        for (%s e : values()) {\n", name))
	gen.emit("            if (e.toString().equals(v)) {\n")
	gen.emit("                return e;\n")
	gen.emit("            }\n")
	gen.emit("        }\n")
	gen.emit(fmt.Sprintf("        throw new IllegalArgumentException(\"Invalid string representation for %s: \" + v);\n", name))
	gen.emit("    }\n")
	gen.emit("}\n")
}

func javaFieldName(n rdl.Identifier) string {
	if n == "default" {
		return "_default"
	}
	return string(n)
}

func (gen *javaModelGenerator) emitStructFields(fields []*rdl.StructFieldDef, name rdl.TypeName, comment string, cName string, bfinal bool) {
	if gen.jackson {
		gen.emit("@JsonSerialize(include = JsonSerialize.Inclusion.NON_DEFAULT)\n")
	}
	sfinal := ""
	if bfinal {
		sfinal = "final "
	}
	gen.emit(fmt.Sprintf("public %sclass %s {\n", sfinal, name))
	if fields != nil {
		fnames := make([]string, 0, len(fields))
		ftypes := make([]string, 0, len(fields))
		for _, f := range fields {
			fname := javaFieldName(f.Name)
			fnames = append(fnames, fname)
			optional := f.Optional
			ftype := javaType(gen.registry, f.Type, optional, f.Items, f.Keys)
			ftypes = append(ftypes, ftype)
			if fname != string(f.Name) {
				gen.emit(fmt.Sprintf("    @com.fasterxml.jackson.annotation.JsonProperty(%q)\n", f.Name))
			}
			if optional {
				gen.emit("    @RdlOptional\n")
			}
			gen.emit(fmt.Sprintf("    public %s %s;\n", ftype, fname))
		}
		gen.emit("\n")
		for i := range fields {
			fname := fnames[i]
			ftype := ftypes[i]
			gen.emit(fmt.Sprintf("    public %s %s(%s %s) {\n        this.%s = %s;\n        return this;\n    }\n", cName, fname, ftype, fname, fname, fname))
		}
		gen.emit("\n")
		gen.emit("    @Override\n    public boolean equals(Object another) {\n")
		gen.emit("        if (this != another) {\n")
		gen.emit(fmt.Sprintf("            if (another == null || another.getClass() != %s.class) {\n", name))
		gen.emit("                return false;\n")
		gen.emit("            }\n")
		gen.emit(fmt.Sprintf("            %s a = (%s) another;\n", name, name))
		for _, f := range fields {
			fname := javaFieldName(f.Name)
			fnames = append(fnames, fname)
			if gen.isFieldPrimitiveType(f) {
				gen.emit(fmt.Sprintf("            if (%s != a.%s) {\n", fname, fname))
			} else {
				gen.emit(fmt.Sprintf("            if (%s == null ? a.%s != null : !%s.equals(a.%s)) {\n", fname, fname, fname, fname))
			}
			gen.emit("                return false;\n")
			gen.emit("            }\n")
		}
		gen.emit("        }\n")
		gen.emit("        return true;\n")
		gen.emit("    }\n")
	}
}