// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.19.1
// source: catalogue.proto

package proto

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

var File_catalogue_proto protoreflect.FileDescriptor

var file_catalogue_proto_rawDesc = []byte{
	0x0a, 0x0f, 0x63, 0x61, 0x74, 0x61, 0x6c, 0x6f, 0x67, 0x75, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x12, 0x09, 0x63, 0x61, 0x74, 0x61, 0x6c, 0x6f, 0x67, 0x75, 0x65, 0x1a, 0x0b, 0x73, 0x74,
	0x6f, 0x63, 0x6b, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x32, 0x4b, 0x0a, 0x09, 0x43, 0x61, 0x74,
	0x61, 0x6c, 0x6f, 0x67, 0x75, 0x65, 0x12, 0x3e, 0x0a, 0x0b, 0x47, 0x65, 0x74, 0x41, 0x72, 0x74,
	0x69, 0x63, 0x6c, 0x65, 0x73, 0x12, 0x16, 0x2e, 0x73, 0x74, 0x6f, 0x63, 0x6b, 0x2e, 0x52, 0x65,
	0x71, 0x75, 0x65, 0x73, 0x74, 0x41, 0x72, 0x74, 0x69, 0x63, 0x6c, 0x65, 0x73, 0x1a, 0x17, 0x2e,
	0x73, 0x74, 0x6f, 0x63, 0x6b, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x41, 0x72,
	0x74, 0x69, 0x63, 0x6c, 0x65, 0x73, 0x42, 0x0a, 0x5a, 0x08, 0x2e, 0x2f, 0x3b, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var file_catalogue_proto_goTypes = []interface{}{
	(*RequestArticles)(nil),  // 0: stock.RequestArticles
	(*ResponseArticles)(nil), // 1: stock.ResponseArticles
}
var file_catalogue_proto_depIdxs = []int32{
	0, // 0: catalogue.Catalogue.GetArticles:input_type -> stock.RequestArticles
	1, // 1: catalogue.Catalogue.GetArticles:output_type -> stock.ResponseArticles
	1, // [1:2] is the sub-list for method output_type
	0, // [0:1] is the sub-list for method input_type
	0, // [0:0] is the sub-list for extension type_name
	0, // [0:0] is the sub-list for extension extendee
	0, // [0:0] is the sub-list for field type_name
}

func init() { file_catalogue_proto_init() }
func file_catalogue_proto_init() {
	if File_catalogue_proto != nil {
		return
	}
	file_stock_proto_init()
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_catalogue_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   0,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_catalogue_proto_goTypes,
		DependencyIndexes: file_catalogue_proto_depIdxs,
	}.Build()
	File_catalogue_proto = out.File
	file_catalogue_proto_rawDesc = nil
	file_catalogue_proto_goTypes = nil
	file_catalogue_proto_depIdxs = nil
}