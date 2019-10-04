// github.com/Avalanch-io/c4/store - is a package for representing generic c4
// storage. A C4 store abstracts away the details of data management allowing
// c4 data consumers and producers to store and retreave c4 identified data
// using the c4 id alone.
//
// A c4 store could represent an object storage bucket, a local filesystem,
// or the agrigation of many c4 stores. A c4 store can also be used to abstract
// processes like encryption, creating distributed copies, and on the fly
// validation.
package store
